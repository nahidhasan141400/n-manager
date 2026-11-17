package monitor

import "testing"

func TestParseCPUAndMemory(t *testing.T) {
	cpu, err := ParseCPUStat("cpu  100 0 50 800 50 0 0 0 0 0\ncpu0 10 0 5 80 5 0 0 0 0 0\n")
	if err != nil {
		t.Fatal(err)
	}
	next, err := ParseCPUStat("cpu  200 0 100 900 50 0 0 0 0 0\n")
	if err != nil {
		t.Fatal(err)
	}
	pct := CPUPercent(cpu, next)
	if pct < 59 || pct > 61 {
		t.Fatalf("cpu percent %v", pct)
	}
	mem, err := ParseMemInfo("MemTotal: 100000 kB\nMemAvailable: 25000 kB\n")
	if err != nil {
		t.Fatal(err)
	}
	if mem.Total != 100000*1024 || mem.Used != 75000*1024 {
		t.Fatalf("%+v", mem)
	}
	a, b, c, err := ParseLoadAvg("0.10 0.20 0.30 1/100 10")
	if err != nil || a != 0.10 || b != 0.20 || c != 0.30 {
		t.Fatalf("%v %v %v %v", a, b, c, err)
	}
}

func TestParseNetAndOS(t *testing.T) {
	text := "Inter-|   Receive                                                |  Transmit\n face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed\n    lo: 10 0 0 0 0 0 0 0 20 0 0 0 0 0 0 0\n  eth0: 1000 0 0 0 0 0 0 0 3000 0 0 0 0 0 0 0\n"
	nets := ParseNetDev(text)
	if nets["eth0"] != [2]uint64{1000, 3000} {
		t.Fatalf("%v", nets)
	}
	pretty, name, ver := ParseOSRelease("NAME=\"Ubuntu\"\nVERSION=\"24.04.1 LTS\"\nPRETTY_NAME=\"Ubuntu 24.04.1 LTS\"\n")
	if pretty == "" || name != "Ubuntu" || ver == "" {
		t.Fatalf("%s %s %s", pretty, name, ver)
	}
}

func TestLiveSample(t *testing.T) {
	info, err := ReadInfo("", "test")
	if err != nil {
		t.Fatal(err)
	}
	if info.Hostname == "" || info.CPUCores < 1 || info.UptimeSeconds <= 0 {
		t.Fatalf("%+v", info)
	}
	disks, err := DiskUsage()
	if err != nil {
		t.Fatal(err)
	}
	foundRoot := false
	for _, d := range disks {
		if d.Mount == "/" && d.Total > 0 {
			foundRoot = true
		}
	}
	if !foundRoot {
		t.Fatalf("root filesystem missing: %+v", disks)
	}
}
