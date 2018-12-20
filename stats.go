package stat

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/mem"
	"github.com/shirou/gopsutil/process"
)

type attrType int

const (
	ATTR_TYPE_ACC_PERIOD attrType = iota
	ATTR_TYPE_ACC_PERMANENT
	ATTR_TYPE_INSTANT
	ATTR_TYPE_MAX
)

type attrNode struct {
	Typ  attrType
	Attr string
	Cnt  int64
	Val  int64
	Max  int64
	Min  int64
	Mean int64
}

func (n *attrNode) clear() {
	if n.Typ == ATTR_TYPE_ACC_PERMANENT {
		return
	}
	aw.sl.Lock()
	delete(aw.nmap, n.Attr)
	aw.sl.Unlock()
}

type attrWorker struct {
	proc     string
	tags     []string
	hostname string

	cpuTag  string
	memTag  string
	heapTag string
	sl      sync.RWMutex
	p       *process.Process
	sock    *net.UDPConn
	nmap    map[string]*attrNode
}

var aw *attrWorker

type Config struct {
	Addr string
	Tags []string
}

/*初始化属性上报
addr:属性收集svr地址,格式"ip:port"
tag:实例（进程，server）标记；如果为空，默认设置为程序名
prefix:表名前缀，不为空，则表名为 prefix-tablename
*/
func Init(cfg *Config) {
	if cfg == nil {
		fmt.Println("Stat config is empty, disable stat.")
		return
	}

	file, _ := exec.LookPath(os.Args[0])
	proc := filepath.Base(file)

	worker := &attrWorker{
		proc: proc,
		tags: cfg.Tags,
	}

	if hostname, err := os.Hostname(); err != nil {
		worker.hostname = "unknown"
	} else {
		worker.hostname = hostname
	}

	udpAddr, err := net.ResolveUDPAddr("udp", cfg.Addr)
	if err != nil {
		fmt.Printf("Stat address error: %v\n", err)
		return
	}
	sock, err := net.DialUDP("udp",
		nil,
		udpAddr,
	)
	if err != nil {
		fmt.Printf("Connect to stat address %v error: %v\n", udpAddr, err)
		return
	}
	worker.sock = sock
	worker.nmap = make(map[string]*attrNode)

	if aw == nil {
		aw = worker
		go loop()
	} else {
		aw = worker
	}

	CPUStat("cpu")
	MemoryStat("mem")
	HeapStat("heap")

	return
}

// 设置属性累加值, 每周期清零
func Add(attr string, v int64) {
	add(attr, v, ATTR_TYPE_ACC_PERIOD)
}

// 设置属性累加值, 不清零
func AddPermanent(attr string, v int64) {
	add(attr, v, ATTR_TYPE_ACC_PERMANENT)
}

func add(attr string, v int64, t attrType) {
	if attr == "" || v == 0 || aw == nil {
		return
	}
	aw.sl.Lock()
	if node, ok := aw.nmap[attr]; ok {
		node.Val += v
		node.Cnt++
		if node.Max < v {
			node.Max = v
		}
		if node.Min > v {
			node.Min = v
		}
	} else {
		aw.nmap[attr] = &attrNode{
			Attr: attr,
			Typ:  t,
			Val:  v,
			Cnt:  1,
			Max:  v,
			Min:  v,
		}
	}
	aw.sl.Unlock()
}

//设置属性瞬时值
//attr:通过"."区分多个属性，格式为 tablename[.attr1]
//上报格式为 瞬时值,次数,最大值,最小值
func Instant(attr string, v int64) {
	if attr == "" || v == 0 || aw == nil {
		return
	}
	aw.sl.Lock()
	if node, ok := aw.nmap[attr]; ok {
		node.Val = v
		node.Cnt++
		if node.Max < v {
			node.Max = v
		}
		if node.Min > v {
			node.Min = v
		}
	} else {
		aw.nmap[attr] = &attrNode{
			Typ:  ATTR_TYPE_INSTANT,
			Attr: attr,
			Val:  v,
			Cnt:  1,
			Max:  v,
			Min:  v,
		}
	}
	aw.sl.Unlock()
}

func Duration(attr string, t time.Time) time.Duration {
	dura := time.Now().Sub(t)
	Add(attr, int64(dura))
	return dura
}

/*
设置进程cpu统计上报，
cpu.sys
cpu.usr
*/
func CPUStat(attr string) {
	if aw != nil {
		if aw.p == nil {
			aw.p, _ = process.NewProcess(int32(os.Getpid()))
		}
		aw.cpuTag = attr
	}
}

/*
设置内存统计上报
*/
func MemoryStat(attr string) {
	if aw != nil {
		if aw.p == nil {
			aw.p, _ = process.NewProcess(int32(os.Getpid()))
		}
		aw.memTag = attr
	}
}

/*
设置内存分配统计上报
*/
func HeapStat(attr string) {
	if aw != nil {
		aw.heapTag = attr
	}
}

func setCpu() {
	if aw.p != nil && aw.cpuTag != "" { //cpu上报
		cputag := aw.cpuTag
		if strings.HasSuffix(cputag, ".cpu") == false {
			cputag += ".cpu"
		}
		if rate, err := aw.p.Percent(0); err == nil {
			Instant(cputag+".usr", int64(rate))
		}
		if rate, err := cpu.Percent(0, false); err == nil {
			Instant(cputag+".sys", int64(rate[0]))
		}
	}
}
func setMemory() {
	if aw.p != nil && aw.memTag != "" {
		if sysMemory, err := mem.VirtualMemory(); err == nil {
			Instant(aw.memTag+".mem.sys.total", int64(sysMemory.Total))
			Instant(aw.memTag+".mem.sys.free", int64(sysMemory.Free))
			Instant(aw.memTag+".mem.sys.usage", int64(sysMemory.UsedPercent))
		}

		if usrMemoryUsage, err := aw.p.MemoryPercent(); err == nil {
			Instant(aw.memTag+".mem.usr.usage", int64(usrMemoryUsage))
		}
		if usrMemoryInfo, err := aw.p.MemoryInfo(); err == nil {
			Instant(aw.memTag+".mem.usr.rss", int64(usrMemoryInfo.RSS))
			Instant(aw.memTag+".mem.usr.vms", int64(usrMemoryInfo.VMS))
			Instant(aw.memTag+".mem.usr.swap", int64(usrMemoryInfo.Swap))
		}
	}
}
func setHeap() {
	if aw.heapTag != "" {
		var memStat runtime.MemStats
		runtime.ReadMemStats(&memStat)
		Instant(aw.heapTag+".heap.sys", int64(memStat.HeapSys))
		Instant(aw.heapTag+".heap.alloc", int64(memStat.HeapAlloc))
		Instant(aw.heapTag+".heap.idle", int64(memStat.HeapIdle))
		Instant(aw.heapTag+".heap.released", int64(memStat.HeapReleased))
		Instant(aw.heapTag+".heap.objects", int64(memStat.HeapObjects))

		Instant(aw.heapTag+".gc.num", int64(memStat.NumGC))
		Instant(aw.heapTag+".gc.pauseNs", int64(memStat.PauseNs[(memStat.NumGC+255)%256]))
	}
}

func loop() {
	var sendBuf bytes.Buffer
	tick := time.Tick(1 * time.Second)

	for {
		select {
		case <-tick:
			if aw == nil {
				break
			}

			setCpu()
			setMemory()
			setHeap()

			aw.sl.Lock()
			al := make([]*attrNode, len(aw.nmap))
			i := 0
			for _, node := range aw.nmap {
				al[i] = node
				i++
			}
			aw.sl.Unlock()

			for _, a := range al {
				attrs := strings.Split(a.Attr, ".")
				if len(attrs) == 0 {
					continue
				}

				sendBuf.Reset()

				sendBuf.WriteString(fmt.Sprintf("%s,ip=$IP,hostname=%s", aw.proc, aw.hostname))

				for i := 0; i < len(aw.tags); i++ {
					sendBuf.WriteString(fmt.Sprintf(",t%d=%s", i+1, aw.tags[i]))
				}

				for i := 0; i < len(attrs); i++ {
					sendBuf.WriteString(fmt.Sprintf(",l%d=%s", i+1, attrs[i]))
				}

				sendBuf.WriteString(fmt.Sprintf(" count=%d,value=%d,max=%d,min=%d\n",
					a.Cnt, a.Val, a.Max, a.Min))

				aw.sock.Write(sendBuf.Bytes())
				a.clear()
			}
		}
	}
}
