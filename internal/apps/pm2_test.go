package apps

import "testing"

func TestParsePM2(t *testing.T) {
	raw := `[{"name":"api","pid":42,"pm2_env":{"pm_id":1,"status":"online","pm_uptime":1,"restart_time":3},"monit":{"memory":1048576,"cpu":2.5}}]`
	got := parsePM2(raw)
	if len(got) != 1 || got[0].Name != "api" || got[0].Status != "online" || got[0].Memory != 1048576 || got[0].Restarts != 3 {
		t.Fatalf("parsed %#v", got)
	}
	if got[0].Uptime <= 0 {
		t.Fatal("expected uptime")
	}
}
