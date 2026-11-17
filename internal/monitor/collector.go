package monitor

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"runtime"
	"sort"
	"sync"
	"time"
)

type Sample struct {
	Time       time.Time  `json:"time"`
	CPUPercent float64    `json:"cpu_percent"`
	CPUCores   int        `json:"cpu_cores"`
	Load1      float64    `json:"load1"`
	Load5      float64    `json:"load5"`
	Load15     float64    `json:"load15"`
	MemTotal   uint64     `json:"mem_total"`
	MemUsed    uint64     `json:"mem_used"`
	MemAvail   uint64     `json:"mem_available"`
	MemPercent float64    `json:"mem_percent"`
	Disks      []Disk     `json:"disks"`
	Net        []NetIface `json:"net"`
	NetRxBps   float64    `json:"net_rx_bps"`
	NetTxBps   float64    `json:"net_tx_bps"`
}

type Point struct {
	T   int64   `json:"t"`
	CPU float64 `json:"cpu"`
	Mem float64 `json:"mem"`
	Rx  float64 `json:"rx"`
	Tx  float64 `json:"tx"`
}

type Snapshot struct {
	Current         Sample  `json:"current"`
	History         []Point `json:"history"`
	RefreshInterval int     `json:"refresh_interval"`
	UpdatedAt       string  `json:"updated_at"`
}

type Collector struct {
	mu       sync.Mutex
	interval time.Duration
	history  []Point
	max      int
	current  Sample
	prevCPU  CPUStat
	prevNet  NetCounters
	prevAt   time.Time
	havePrev bool
	publicIP string
	publicAt time.Time
	version  string
	stop     chan struct{}
	bump     chan struct{}
}

func NewCollector(interval time.Duration, appVersion string) *Collector {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	return &Collector{
		interval: interval,
		max:      360,
		history:  []Point{},
		version:  appVersion,
		stop:     make(chan struct{}),
		bump:     make(chan struct{}, 1),
	}
}

func (c *Collector) SetInterval(d time.Duration) {
	if d < time.Second {
		d = time.Second
	}
	c.mu.Lock()
	c.interval = d
	c.mu.Unlock()
	select {
	case c.bump <- struct{}{}:
	default:
	}
}

func (c *Collector) Start() {
	c.prime()
	go c.loop()
	go c.publicLoop()
}

func (c *Collector) Stop() {
	select {
	case <-c.stop:
	default:
		close(c.stop)
	}
}

func (c *Collector) prime() {
	c.sample(false)
	time.Sleep(200 * time.Millisecond)
	c.sample(true)
}

func (c *Collector) loop() {
	c.mu.Lock()
	interval := c.interval
	c.mu.Unlock()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-c.stop:
			return
		case <-c.bump:
			c.mu.Lock()
			interval = c.interval
			c.mu.Unlock()
			ticker.Reset(interval)
		case <-ticker.C:
			c.sample(true)
		}
	}
}

func (c *Collector) sample(record bool) {
	cpuText, err := os.ReadFile("/proc/stat")
	if err != nil {
		return
	}
	cpu, err := ParseCPUStat(string(cpuText))
	if err != nil {
		return
	}
	memText, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return
	}
	mem, err := ParseMemInfo(string(memText))
	if err != nil {
		return
	}
	load1, load5, load15 := 0.0, 0.0, 0.0
	if b, err := os.ReadFile("/proc/loadavg"); err == nil {
		load1, load5, load15, _ = ParseLoadAvg(string(b))
	}
	netText, _ := os.ReadFile("/proc/net/dev")
	nets := ParseNetDev(string(netText))
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	var cpuPct float64
	if c.havePrev {
		cpuPct = CPUPercent(c.prevCPU, cpu)
	}
	elapsed := now.Sub(c.prevAt).Seconds()
	ifaces := make([]NetIface, 0, len(nets))
	var rxBps, txBps float64
	for name, cur := range nets {
		var rps, tps float64
		if c.havePrev && elapsed > 0 {
			prev := c.prevNet[name]
			if cur[0] >= prev[0] {
				rps = float64(cur[0]-prev[0]) / elapsed
			}
			if cur[1] >= prev[1] {
				tps = float64(cur[1]-prev[1]) / elapsed
			}
		}
		ifaces = append(ifaces, NetIface{Name: name, Rx: cur[0], Tx: cur[1], RxBps: rps, TxBps: tps})
		if name != "lo" {
			rxBps += rps
			txBps += tps
		}
	}
	disks, _ := DiskUsage()
	sort.Slice(ifaces, func(i, j int) bool { return ifaces[i].Name < ifaces[j].Name })
	sample := Sample{
		Time: now, CPUPercent: cpuPct, CPUCores: runtime.NumCPU(),
		Load1: load1, Load5: load5, Load15: load15,
		MemTotal: mem.Total, MemUsed: mem.Used, MemAvail: mem.Available, MemPercent: mem.Percent,
		Disks: disks, Net: ifaces, NetRxBps: rxBps, NetTxBps: txBps,
	}
	c.current = sample
	c.prevCPU = cpu
	c.prevNet = nets
	c.prevAt = now
	c.havePrev = true
	if record {
		c.history = append(c.history, Point{T: now.UnixMilli(), CPU: cpuPct, Mem: mem.Percent, Rx: rxBps, Tx: txBps})
		if len(c.history) > c.max {
			c.history = c.history[len(c.history)-c.max:]
		}
	}
}

func (c *Collector) Snapshot() Snapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	history := append([]Point(nil), c.history...)
	if history == nil {
		history = []Point{}
	}
	updated := c.current.Time
	if updated.IsZero() {
		updated = time.Now()
	}
	return Snapshot{
		Current:         c.current,
		History:         history,
		RefreshInterval: int(c.interval / time.Second),
		UpdatedAt:       updated.UTC().Format(time.RFC3339),
	}
}

func (c *Collector) Info() (Info, error) {
	c.mu.Lock()
	ip := c.publicIP
	version := c.version
	c.mu.Unlock()
	return ReadInfo(ip, version)
}

func (c *Collector) publicLoop() {
	c.fetchPublic()
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-c.stop:
			return
		case <-ticker.C:
			c.fetchPublic()
		}
	}
}

func (c *Collector) fetchPublic() {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.ipify.org", nil)
	if err != nil {
		return
	}
	req.Header.Set("User-Agent", "n-manager")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64))
	if err != nil || resp.StatusCode != 200 {
		return
	}
	ip := stringsTrim(string(body))
	if net.ParseIP(ip) == nil {
		return
	}
	c.mu.Lock()
	c.publicIP = ip
	c.publicAt = time.Now()
	c.mu.Unlock()
}

func stringsTrim(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r' || s[len(s)-1] == ' ') {
		s = s[:len(s)-1]
	}
	return s
}
