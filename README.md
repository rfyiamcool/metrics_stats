# metrics_stats

simple push metrics stats to influxdb with udp .

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

    select{}
}
```