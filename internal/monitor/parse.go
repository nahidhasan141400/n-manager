package monitor

import (
	"bufio"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"
)

type Disk struct {
	Mount      string  `json:"mount"`
	Filesystem string  `json:"filesystem"`
	Total      uint64  `json:"total"`
	Used       uint64  `json:"used"`
	Available  uint64  `json:"available"`
	Percent    float64 `json:"percent"`
}

type NetIface struct {
	Name  string  `json:"name"`
	Rx    uint64  `json:"rx"`
	Tx    uint64  `json:"tx"`
	RxBps float64 `json:"rx_bps"`
	TxBps float64 `json:"tx_bps"`
}

type CPUStat struct {
	Idle  uint64
	Total uint64
}

type Info struct {
	Hostname      string   `json:"hostname"`
	OS            string   `json:"os"`
	Distro        string   `json:"distro"`
	DistroVersion string   `json:"distro_version"`
	Kernel        string   `json:"kernel"`
	CPUModel      string   `json:"cpu_model"`
	CPUCores      int      `json:"cpu_cores"`
	UptimeSeconds float64  `json:"uptime_seconds"`
	PrivateIPs    []string `json:"private_ips"`
	PublicIP      string   `json:"public_ip,omitempty"`
	AppVersion    string   `json:"app_version"`
}

func ParseCPUStat(text string) (CPUStat, error) {
	for _, line := range strings.Split(text, "\n") {
		if !strings.HasPrefix(line, "cpu ") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 5 {
			return CPUStat{}, fmt.Errorf("short cpu line")
		}
		var nums []uint64
		for _, f := range fields[1:] {
			n, err := strconv.ParseUint(f, 10, 64)
			if err != nil {
				return CPUStat{}, err
			}
			nums = append(nums, n)
		}
		if len(nums) > 8 {
			nums = nums[:8]
		}
		var idle, total uint64
		for i, n := range nums {
			total += n
			if i == 3 || i == 4 {
				idle += n
			}
		}
		return CPUStat{Idle: idle, Total: total}, nil
	}
	return CPUStat{}, fmt.Errorf("cpu line not found")
}

func CPUPercent(prev, next CPUStat) float64 {
	totald := float64(next.Total - prev.Total)
	idled := float64(next.Idle - prev.Idle)
	if totald <= 0 {
		return 0
	}
	pct := (totald - idled) / totald * 100
	if pct < 0 {
		return 0
	}
	if pct > 100 {
		return 100
	}
	return pct
}

type Memory struct {
	Total     uint64
	Available uint64
	Used      uint64
	Percent   float64
}

func ParseMemInfo(text string) (Memory, error) {
	vals := map[string]uint64{}
	for _, line := range strings.Split(text, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		key := strings.TrimSuffix(fields[0], ":")
		n, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}
		vals[key] = n * 1024
	}
	total := vals["MemTotal"]
	if total == 0 {
		return Memory{}, fmt.Errorf("MemTotal missing")
	}
	avail := vals["MemAvailable"]
	if avail == 0 {
		avail = vals["MemFree"]
	}
	used := uint64(0)
	if total > avail {
		used = total - avail
	}
	return Memory{Total: total, Available: avail, Used: used, Percent: float64(used) / float64(total) * 100}, nil
}

func ParseLoadAvg(text string) (float64, float64, float64, error) {
	fields := strings.Fields(text)
	if len(fields) < 3 {
		return 0, 0, 0, fmt.Errorf("short loadavg")
	}
	a, err1 := strconv.ParseFloat(fields[0], 64)
	b, err2 := strconv.ParseFloat(fields[1], 64)
	c, err3 := strconv.ParseFloat(fields[2], 64)
	if err1 != nil || err2 != nil || err3 != nil {
		return 0, 0, 0, fmt.Errorf("parse loadavg")
	}
	return a, b, c, nil
}

func ParseUptime(text string) (float64, error) {
	fields := strings.Fields(text)
	if len(fields) < 1 {
		return 0, fmt.Errorf("short uptime")
	}
	return strconv.ParseFloat(fields[0], 64)
}

type NetCounters map[string][2]uint64

func ParseNetDev(text string) NetCounters {
	out := NetCounters{}
	for _, line := range strings.Split(text, "\n") {
		if !strings.Contains(line, ":") {
			continue
		}
		name, rest, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		fields := strings.Fields(rest)
		if len(fields) < 9 {
			continue
		}
		rx, err1 := strconv.ParseUint(fields[0], 10, 64)
		tx, err2 := strconv.ParseUint(fields[8], 10, 64)
		if err1 != nil || err2 != nil {
			continue
		}
		out[name] = [2]uint64{rx, tx}
	}
	return out
}

func ParseOSRelease(text string) (pretty, name, version string) {
	vals := map[string]string{}
	for _, line := range strings.Split(text, "\n") {
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		vals[k] = strings.Trim(v, `"`)
	}
	return vals["PRETTY_NAME"], vals["NAME"], vals["VERSION"]
}

func ReadInfo(publicIP, appVersion string) (Info, error) {
	host, _ := os.Hostname()
	pretty, name, ver := "", "", ""
	if b, err := os.ReadFile("/etc/os-release"); err == nil {
		pretty, name, ver = ParseOSRelease(string(b))
	}
	kernel := ""
	if b, err := os.ReadFile("/proc/sys/kernel/osrelease"); err == nil {
		kernel = strings.TrimSpace(string(b))
	}
	model := cpuModel()
	uptime := 0.0
	if b, err := os.ReadFile("/proc/uptime"); err == nil {
		uptime, _ = ParseUptime(string(b))
	}
	if pretty == "" {
		pretty = name
	}
	ips := privateIPs()
	if ips == nil {
		ips = []string{}
	}
	return Info{
		Hostname:      host,
		OS:            pretty,
		Distro:        name,
		DistroVersion: ver,
		Kernel:        kernel,
		CPUModel:      model,
		CPUCores:      runtime.NumCPU(),
		UptimeSeconds: uptime,
		PrivateIPs:    ips,
		PublicIP:      publicIP,
		AppVersion:    appVersion,
	}, nil
}

func cpuModel() string {
	f, err := os.Open("/proc/cpuinfo")
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "model name") {
			_, v, ok := strings.Cut(line, ":")
			if ok {
				return strings.TrimSpace(v)
			}
		}
	}
	return ""
}

func privateIPs() []string {
	// Imported in collector to avoid an import cycle with net in tests of parsers.
	return readPrivateIPs()
}

func DiskUsage() ([]Disk, error) {
	f, err := os.Open("/proc/mounts")
	if err != nil {
		return nil, err
	}
	defer f.Close()
	skipType := map[string]struct{}{
		"proc": {}, "sysfs": {}, "devtmpfs": {}, "devpts": {}, "tmpfs": {},
		"cgroup": {}, "cgroup2": {}, "overlay": {}, "squashfs": {}, "securityfs": {},
		"pstore": {}, "bpf": {}, "tracefs": {}, "debugfs": {}, "fusectl": {},
		"configfs": {}, "ramfs": {}, "autofs": {}, "mqueue": {}, "hugetlbfs": {},
		"efivarfs": {}, "nsfs": {}, "rpc_pipefs": {}, "binfmt_misc": {},
	}
	var out []Disk
	seen := map[string]struct{}{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 3 {
			continue
		}
		fsType := fields[2]
		mount := fields[1]
		mount = unescapeMount(mount)
		if _, skip := skipType[fsType]; skip {
			continue
		}
		if strings.HasPrefix(mount, "/proc") || strings.HasPrefix(mount, "/sys") || strings.HasPrefix(mount, "/dev") || strings.HasPrefix(mount, "/run") || strings.HasPrefix(mount, "/snap") {
			continue
		}
		key := fields[0] + "@" + mount
		if _, ok := seen[key]; ok {
			continue
		}
		var st syscall.Statfs_t
		if err := syscall.Statfs(mount, &st); err != nil || st.Blocks == 0 {
			continue
		}
		bsize := uint64(st.Bsize)
		total := st.Blocks * bsize
		free := st.Bfree * bsize
		avail := st.Bavail * bsize
		used := uint64(0)
		if total > free {
			used = total - free
		}
		pct := 0.0
		if total > 0 {
			pct = float64(used) / float64(total) * 100
		}
		seen[key] = struct{}{}
		out = append(out, Disk{
			Mount: mount, Filesystem: fsType, Total: total, Used: used, Available: avail, Percent: pct,
		})
	}
	if out == nil {
		out = []Disk{}
	}
	return out, sc.Err()
}

func unescapeMount(s string) string {
	replacer := strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`)
	return replacer.Replace(s)
}
