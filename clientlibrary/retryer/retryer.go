/*
 * Copyright (c) 2018 VMware, Inc.
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy of this software and
 * associated documentation files (the "Software"), to deal in the Software without restriction, including
 * without limitation the rights to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is furnished to do
 * so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in all copies or substantial
 * portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED, INCLUDING BUT
 * NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
 * IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY,
 * WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE
 * SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
 */

package retryer

import (
	"context"
	"errors"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	"github.com/aws/aws-sdk-go-v2/service/kinesis/types"
)

// noopRateLimiter implements retry.RateLimiter with no token accounting,
// disabling the SDK retry-quota token bucket entirely.
type noopRateLimiter struct{}

func (noopRateLimiter) GetToken(_ context.Context, _ uint) (func() error, error) {
	return func() error { return nil }, nil
}
func (noopRateLimiter) AddTokens(_ uint) error { return nil }

// NewThrottlePassthroughRetryer returns an aws.Retryer suitable for Kinesis
// clients used with KCL. Pass it via awsConfig.WithRetryer when building the
// kinesis client:
//
//	cfg, _ := awsConfig.LoadDefaultConfig(ctx,
//	    awsConfig.WithRetryer(func() aws.Retryer {
//	        return retryer.NewThrottlePassthroughRetryer()
//	    }),
//	)
//	kinesisClient := kinesis.NewFromConfig(cfg)
//	streamWorker.WithKinesis(kinesisClient)
//
// Two behaviours differ from the SDK default:
//
//  1. ProvisionedThroughputExceededException is NOT retried by the SDK.
//     KCL's own polling loop handles Kinesis throttling with a one-second
//     back-off and a configurable retry count. SDK retries for the same error
//     amplify actual TPS hitting Kinesis (each KCL call becomes up to 3 real
//     API calls) and accelerate token-bucket exhaustion.
//
//  2. The SDK retry-quota token bucket is disabled.
//     With throttle errors no longer retried by the SDK, the bucket would
//     still drain on other transient errors and eventually block retries that
//     are genuinely useful (e.g. network blips). Disabling it lets MaxAttempts
//     alone govern retry limits for all remaining retryable errors.
func NewThrottlePassthroughRetryer() aws.Retryer {
	return retry.NewStandard(func(o *retry.StandardOptions) {
		o.RateLimiter = noopRateLimiter{}

		// Prepend before DefaultRetryables, which already lists
		// ProvisionedThroughputExceededException as retryable.
		o.Retryables = append(
			[]retry.IsErrorRetryable{
				retry.IsErrorRetryableFunc(func(err error) aws.Ternary {
					var t *types.ProvisionedThroughputExceededException
					if errors.As(err, &t) {
						return aws.FalseTernary
					}
					return aws.UnknownTernary
				}),
			},
			o.Retryables...,
		)
	})
}
