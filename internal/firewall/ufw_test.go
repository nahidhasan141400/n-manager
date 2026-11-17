package firewall

import "testing"

func TestParseStatus(t *testing.T) {
	verbose := "Status: active\nLogging: on (low)\nDefault: deny (incoming), allow (outgoing), disabled (routed)\n"
	numbered := "" +
		"Status: active\n\n" +
		"     To                         Action      From\n" +
		"     --                         ------      ----\n" +
		"[ 1] 22/tcp                     ALLOW IN    Anywhere\n" +
		"[ 2] 80/tcp                     ALLOW IN    Anywhere                   # HTTP\n" +
		"[ 3] 22/tcp (v6)                ALLOW IN    Anywhere (v6)\n"
	status := Parse(verbose, numbered)
	if !status.Active || status.DefaultIncoming != "deny" || status.DefaultOutgoing != "allow" {
		t.Fatalf("status = %+v", status)
	}
	if len(status.Rules) != 3 {
		t.Fatalf("rules = %+v", status.Rules)
	}
	if status.Rules[1].Comment != "HTTP" || status.Rules[1].To != "80/tcp" {
		t.Fatalf("rule = %+v", status.Rules[1])
	}
	if !status.Rules[2].V6 || sshAllows(status.Rules, 22) != 1 {
		t.Fatalf("ssh rules = %+v", status.Rules)
	}
}

func TestParseAdded(t *testing.T) {
	text := "Added user rules (see 'ufw status' for running firewall):\n" +
		"ufw allow 22/tcp\n" +
		"ufw allow 80/tcp comment 'HTTP'\n" +
		"ufw allow from 203.0.113.10 to any port 443 proto tcp\n"
	rules := ParseAdded(text)
	if len(rules) != 3 {
		t.Fatalf("rules = %+v", rules)
	}
	if rules[1].To != "80/tcp" || rules[1].Comment != "HTTP" || rules[1].From != "Anywhere" {
		t.Fatalf("http = %+v", rules[1])
	}
	if rules[2].To != "443/tcp" || rules[2].From != "203.0.113.10" {
		t.Fatalf("limited = %+v", rules[2])
	}
	if sshAllows(rules, 22) != 1 {
		t.Fatalf("ssh = %+v", rules)
	}
	if got := join(rules[1].DeleteArgs); got != "allow 80/tcp comment HTTP" {
		t.Fatalf("delete = %s", got)
	}
	inserted := ParseAdded("ufw insert 1 allow 22/tcp comment 'SSH'\nufw deny out 53/udp\n")
	if len(inserted) != 2 || !sshReachable(inserted, 22) || join(inserted[0].DeleteArgs) != "allow 22/tcp comment SSH" {
		t.Fatalf("inserted = %+v", inserted)
	}
	if inserted[1].Action != "DENY OUT" || inserted[1].To != "53/udp" {
		t.Fatalf("outgoing = %+v", inserted[1])
	}
}

func TestRuleArgs(t *testing.T) {
	out, err := RuleArgs("deny", "out", 53, "udp", "any", "", 22)
	if err != nil {
		t.Fatal(err)
	}
	if got := join(out); got != "deny out 53/udp" {
		t.Fatalf("args = %s", got)
	}
	if _, err := RuleArgs("limit", "in", 22, "tcp", "any", "", 22); err == nil {
		t.Fatal("expected the ssh port to stay allowed")
	}
	mysql, err := RuleArgs("deny", "in", 3306, "tcp", "any", "", 22)
	if err != nil {
		t.Fatal(err)
	}
	if got := join(mysql); got != "deny 3306/tcp" {
		t.Fatalf("args = %s", got)
	}
}

func TestConfigValue(t *testing.T) {
	got := setConfigValue("# comment\nIPV6=yes\n", "IPV6", "no")
	if got != "# comment\nIPV6=no\n" {
		t.Fatalf("config = %q", got)
	}
	if loggingLevel("on (medium)") != "medium" || loggingLevel("off") != "off" {
		t.Fatal("logging level")
	}
	if !ipv6Enabled("IPV6=yes\n") || ipv6Enabled("IPV6=no\n") {
		t.Fatal("ipv6 flag")
	}
}

func TestSSHStaysReachable(t *testing.T) {
	allow := Rule{Number: 1, To: "22/tcp", Action: "ALLOW IN", From: "Anywhere"}
	limited := Rule{Number: 2, To: "22/tcp", Action: "ALLOW IN", From: "203.0.113.10"}
	deny := Rule{Number: 3, To: "22/tcp", Action: "DENY IN", From: "Anywhere"}
	if !sshReachable([]Rule{allow, deny}, 22) {
		t.Fatal("allow before deny should stay reachable")
	}
	if sshReachable([]Rule{deny, allow}, 22) {
		t.Fatal("deny before allow should block ssh")
	}
	if sshReachable([]Rule{limited}, 22) {
		t.Fatal("an allow from one address should not count")
	}
	if sshReachable(withoutRule([]Rule{allow, limited}, allow.Number), 22) {
		t.Fatal("removing the anywhere allow should block ssh")
	}
}

func TestEditRule(t *testing.T) {
	ssh := Rule{Number: 1, To: "22/tcp", Action: "ALLOW IN", From: "Anywhere", Comment: "SSH"}
	web := Rule{Number: 2, To: "80/tcp", Action: "ALLOW IN", From: "Anywhere", Comment: "HTTP"}
	if !describes(web, "allow", "in", 80, "tcp", "any", "HTTP") {
		t.Fatal("expected the http rule to match")
	}
	edited := append(withoutMatching([]Rule{ssh, web}, web), previewRule("allow", "in", 8080, "tcp", "any", "App"))
	if !sshReachable(edited, 22) || len(edited) != 2 {
		t.Fatalf("edited = %+v", edited)
	}
	moved := append(withoutMatching([]Rule{ssh, web}, ssh), previewRule("allow", "in", 2222, "tcp", "any", "SSH"))
	if sshReachable(moved, 22) {
		t.Fatal("moving ssh to another port should not leave port 22 allowed")
	}
}

func TestPolicies(t *testing.T) {
	in, out := policies("DEFAULT_INPUT_POLICY=\"DROP\"\nDEFAULT_OUTPUT_POLICY=\"ACCEPT\"\n")
	if in != "deny" || out != "allow" {
		t.Fatalf("policies = %s %s", in, out)
	}
}

func TestAllowArgs(t *testing.T) {
	args, err := AllowArgs(443, "tcp", "any", "HTTPS")
	if err != nil {
		t.Fatal(err)
	}
	if got := join(args); got != "allow 443/tcp comment HTTPS" {
		t.Fatalf("args = %s", got)
	}
	limited, err := AllowArgs(22, "tcp", "203.0.113.10", "")
	if err != nil {
		t.Fatal(err)
	}
	if got := join(limited); got != "allow from 203.0.113.10 to any port 22 proto tcp" {
		t.Fatalf("args = %s", got)
	}
	if _, err := AllowArgs(3306, "tcp", "any", ""); err == nil {
		t.Fatal("expected MySQL port to be rejected")
	}
}

func TestInactiveStatus(t *testing.T) {
	status := Parse("Status: inactive\n", "Status: inactive\n")
	if status.Active || len(status.Rules) != 0 {
		t.Fatalf("status = %+v", status)
	}
}

func join(args []string) string {
	out := ""
	for i, arg := range args {
		if i > 0 {
			out += " "
		}
		out += arg
	}
	return out
}
