package sppAdapterWeb

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/constants"
	utils "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/utils_grpc"
)

func asyncWriteBidDataToRedis(
	ctx context.Context,
	timeout time.Duration,
	redisClient *redis.Client,
	globalId string,
	bidReqData []byte,
	countryISO string,
) {
	bgCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)

	go func() {
		defer cancel()

		if err := writeBidDataToRedis(bgCtx, redisClient, globalId, bidReqData, countryISO); err != nil {
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
				log.Printf("background redis writes canceled for globalId %s: %v", globalId, err)
				return
			}

			log.Printf("background redis writes failed for globalId %s: %v", globalId, err)
			return
		}

		log.Printf("background redis writes succeeded for globalId %s", globalId)
	}()
}

func writeBidDataToRedis(
	ctx context.Context,
	redisClient *redis.Client,
	globalId string,
	bidReqData []byte,
	countryISO string,
) error {
	var errs []error

	if err := utils.WriteJsonToRedis(ctx, redisClient, globalId, constants.BID_REQUEST_COLUMN, bidReqData); err != nil {
		errs = append(errs, fmt.Errorf("bid request: %w", err))
	}

	if err := utils.WriteStringToRedis(ctx, redisClient, globalId, constants.GEO_COLUMN, countryISO); err != nil {
		errs = append(errs, fmt.Errorf("geo: %w", err))
	}

	if err := utils.WriteStringToRedis(ctx, redisClient, globalId, constants.RESULT_COLUMN, constants.UNSUCCESS); err != nil {
		errs = append(errs, fmt.Errorf("result: %w", err))
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	return nil
}
