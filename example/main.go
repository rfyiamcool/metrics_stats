package main

import (
	"time"

	stats "github.com/rfyiamcool/metrics_stats"
)

func main() {
	conf := stats.Config{
		Addr: "udp_addr:port",
		Tags: []string{"api"},
	}
	stats.Init(&conf)

	go func() {
		for {
			time.Sleep(1 * time.Second)
			stats.Add("db.err", 1)
		}
	}()

	go func() {
		for {
			time.Sleep(1 * time.Second)
			stats.AddPermanent("insert.qps", 1)
		}
	}()

	select {}
}
