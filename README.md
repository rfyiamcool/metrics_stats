# metrics_stats

Simple push metrics stats to influxdb with udp, push once a second by default.

![](stats.jpg)

## Usage

```
import (
    stats "github.com/rfyiamcool/metrics_stats"
)

func main() {
    conf := stats.Config{
        Addr: "udp_addr:port",
        Tags: []string{"api"},
    }
    stat.Init(conf)

	go func(){
		for {
			time.Sleep(1 * time.Second)
			stats.Add("db.err", 1)
		}
	}()

	go func(){
		for {
			time.Sleep(1 * time.Second)
			stats.AddPermanent("insert.qps", 1)
		}
	}()

    select{}
}
```