package main

import (
	"context"
	"log"

	"github.com/westphae/go-iio/icm20948"
)

// imuConfig parameterizes runIMU.
type imuConfig struct {
	HzTrigger    int     // hrtimer frequency; max ODR (100 Hz) for mag oversampling
	AccelDLPFHz  float64 // 0 = leave kernel default
	GyroDLPFHz   float64 // 0 = leave kernel default
	AccelScaleG  int     // 0 = leave kernel default
	GyroScaleDps int     // 0 = leave kernel default
}

// runIMU opens the ICM20948 via go-iio and streams every sample at the chip's
// max ODR. Decimation/averaging happens downstream — this layer just delivers
// every fresh sample the chip produces. The AK09916 mag conversion is fixed
// at 100 Hz inside the kernel driver, so HzTrigger=100 reads every fresh mag
// sample exactly once.
//
// On open failure the function logs and returns; ctx cancellation closes the
// stream. The returned channel is closed when the goroutine exits.
func runIMU(ctx context.Context, cfg imuConfig) (<-chan icm20948.Sample, error) {
	opts := []icm20948.Option{}
	if cfg.AccelScaleG > 0 {
		opts = append(opts, icm20948.WithAccelScale(cfg.AccelScaleG))
	}
	if cfg.GyroScaleDps > 0 {
		opts = append(opts, icm20948.WithGyroScale(cfg.GyroScaleDps))
	}
	if cfg.AccelDLPFHz > 0 {
		opts = append(opts, icm20948.WithAccelDLPFHz(cfg.AccelDLPFHz))
	}
	if cfg.GyroDLPFHz > 0 {
		opts = append(opts, icm20948.WithGyroDLPFHz(cfg.GyroDLPFHz))
	}
	dev, err := icm20948.Open(opts...)
	if err != nil {
		return nil, err
	}
	samples, err := dev.Stream(ctx, icm20948.StreamOptions{
		FrequencyHz:   cfg.HzTrigger,
		ChannelBuffer: 4 * cfg.HzTrigger,
	})
	if err != nil {
		dev.Close()
		return nil, err
	}
	out := make(chan icm20948.Sample, 4*cfg.HzTrigger)
	go func() {
		defer close(out)
		defer dev.Close()
		for s := range samples {
			select {
			case out <- s:
			case <-ctx.Done():
				return
			}
		}
		if ctx.Err() == nil {
			log.Printf("compass: imu stream ended unexpectedly")
		}
	}()
	return out, nil
}
