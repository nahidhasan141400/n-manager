package monitor

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"time"

	"n-manager/internal/remote"
)

// Series keeps a short metric history for each selected server.
type Series struct {
	mu    sync.Mutex
	by    map[string]*track
	max   int
	every time.Duration
}

type track struct {
	prevCPU CPUStat
	prevNet NetCounters
	prevAt  time.Time
	have    bool
	history []Point
	current Sample
}

func (s *Series) init() {
	if s.by == nil {
		s.by = map[string]*track{}
		s.max = 360
		s.every = 5 * time.Second
	}
}

func (s *Series) Info(ctx context.Context, version string) (Info, error) {
	host, _ := remote.Run(ctx, "hostname", nil, "")
	osRelease, _ := remote.ReadFile(ctx, "/etc/os-release")
	kernel, _ := remote.Run(ctx, "uname", []string{"-r"}, "")
	cpuinfo, _ := remote.ReadFile(ctx, "/proc/cpuinfo")
	uptimeText, _ := remote.ReadFile(ctx, "/proc/uptime")
	ips, _ := remote.Run(ctx, "hostname", []string{"-I"}, "")
	distro, distroVersion := releaseValue(string(osRelease), "NAME"), releaseValue(string(osRelease), "VERSION_ID")
	uptime, _ := ParseUptime(string(uptimeText))
	cores := 0
	if out, err := remote.Run(ctx, "nproc", nil, ""); err == nil {
		cores, _ = strconv.Atoi(strings.TrimSpace(out))
	}
	return Info{
		Hostname:      strings.TrimSpace(host),
		OS:            "linux",
		Distro:        distro,
		DistroVersion: distroVersion,
		Kernel:        strings.TrimSpace(kernel),
		CPUModel:      cpuModelName(string(cpuinfo)),
		CPUCores:      cores,
		UptimeSeconds: uptime,
		PrivateIPs:    strings.Fields(ips),
		AppVersion:    version,
	}, nil
}

func (s *Series) Snapshot(ctx context.Context, id string) (Snapshot, error) {
	s.mu.Lock()
	s.init()
	tr := s.by[id]
	if tr == nil {
		tr = &track{history: []Point{}}
		s.by[id] = tr
	}
	s.mu.Unlock()

	sample, err := sampleHost(ctx, tr)
	if err != nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		if tr.current.Time.IsZero() {
			return Snapshot{}, err
		}
		history := append([]Point(nil), tr.history...)
		if history == nil {
			history = []Point{}
		}
		return Snapshot{
			Current: tr.current, History: history, RefreshInterval: int(s.every / time.Second),
			UpdatedAt: tr.current.Time.UTC().Format(time.RFC3339),
		}, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tr.current = sample
	tr.history = append(tr.history, Point{
		T: sample.Time.Unix(), CPU: sample.CPUPercent, Mem: sample.MemPercent, Rx: sample.NetRxBps, Tx: sample.NetTxBps,
	})
	if len(tr.history) > s.max {
		tr.history = tr.history[len(tr.history)-s.max:]
	}
	history := append([]Point(nil), tr.history...)
	return Snapshot{
		Current: sample, History: history, RefreshInterval: int(s.every / time.Second),
		UpdatedAt: sample.Time.UTC().Format(time.RFC3339),
	}, nil
}

func sampleHost(ctx context.Context, tr *track) (Sample, error) {
	now := time.Now()
	statText, err := remote.ReadFile(ctx, "/proc/stat")
	if err != nil {
		return Sample{}, err
	}
	cpu, err := ParseCPUStat(string(statText))
	if err != nil {
		return Sample{}, err
	}
	memText, err := remote.ReadFile(ctx, "/proc/meminfo")
	if err != nil {
		return Sample{}, err
	}
	mem, err := ParseMemInfo(string(memText))
	if err != nil {
		return Sample{}, err
	}
	loadText, _ := remote.ReadFile(ctx, "/proc/loadavg")
	l1, l5, l15, _ := ParseLoadAvg(string(loadText))
	netText, _ := remote.ReadFile(ctx, "/proc/net/dev")
	nets := ParseNetDev(string(netText))
	dfText, _ := remote.Run(ctx, "df", []string{"-B1", "-P"}, "")
	disks := parseDF(dfText)
	var cpuPct, rx, tx float64
	ifaces := []NetIface{}
	if tr.have && now.After(tr.prevAt) {
		dt := now.Sub(tr.prevAt).Seconds()
		if dt > 0 && cpu.Total > tr.prevCPU.Total {
			idle := float64(cpu.Idle - tr.prevCPU.Idle)
			total := float64(cpu.Total - tr.prevCPU.Total)
			cpuPct = (1 - idle/total) * 100
			if cpuPct < 0 {
				cpuPct = 0
			}
		}
		var rxB, txB uint64
		for name, cur := range nets {
			prev := tr.prevNet[name]
			item := NetIface{Name: name, Rx: cur[0], Tx: cur[1]}
			if cur[0] >= prev[0] {
				item.RxBps = float64(cur[0]-prev[0]) / dt
				rxB += cur[0] - prev[0]
			}
			if cur[1] >= prev[1] {
				item.TxBps = float64(cur[1]-prev[1]) / dt
				txB += cur[1] - prev[1]
			}
			ifaces = append(ifaces, item)
		}
		rx = float64(rxB) / dt
		tx = float64(txB) / dt
	}
	tr.prevCPU = cpu
	tr.prevNet = nets
	tr.prevAt = now
	tr.have = true
	cores := 0
	if out, nerr := remote.Run(ctx, "nproc", nil, ""); nerr == nil {
		cores, _ = strconv.Atoi(strings.TrimSpace(out))
	}
	return Sample{
		Time: now, CPUPercent: cpuPct, CPUCores: cores,
		Load1: l1, Load5: l5, Load15: l15,
		MemTotal: mem.Total, MemUsed: mem.Used, MemAvail: mem.Available, MemPercent: mem.Percent,
		Disks: disks, Net: ifaces, NetRxBps: rx, NetTxBps: tx,
	}, nil
}

func parseDF(text string) []Disk {
	var out []Disk
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if i == 0 || strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 6 {
			continue
		}
		total, _ := strconv.ParseUint(fields[1], 10, 64)
		used, _ := strconv.ParseUint(fields[2], 10, 64)
		avail, _ := strconv.ParseUint(fields[3], 10, 64)
		pct := 0.0
		if total > 0 {
			pct = float64(used) / float64(total) * 100
		}
		out = append(out, Disk{
			Filesystem: fields[0], Total: total, Used: used, Available: avail, Percent: pct, Mount: fields[len(fields)-1],
		})
	}
	if out == nil {
		return []Disk{}
	}
	return out
}

func releaseValue(text, key string) string {
	for _, line := range strings.Split(text, "\n") {
		name, value, ok := strings.Cut(line, "=")
		if ok && name == key {
			return strings.Trim(value, `"`)
		}
	}
	return ""
}

func cpuModelName(text string) string {
	for _, line := range strings.Split(text, "\n") {
		name, value, ok := strings.Cut(line, ":")
		if ok && strings.TrimSpace(name) == "model name" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
