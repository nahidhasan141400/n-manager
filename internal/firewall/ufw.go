package firewall

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"n-manager/internal/remote"
	"n-manager/internal/validate"
)

type Rule struct {
	Number     int      `json:"number"`
	To         string   `json:"to"`
	Action     string   `json:"action"`
	From       string   `json:"from"`
	Comment    string   `json:"comment"`
	V6         bool     `json:"v6"`
	DeleteArgs []string `json:"-"`
}

type Status struct {
	Installed       bool   `json:"installed"`
	Active          bool   `json:"active"`
	Logging         string `json:"logging"`
	DefaultIncoming string `json:"default_incoming"`
	DefaultOutgoing string `json:"default_outgoing"`
	SSHPort         int    `json:"ssh_port,omitempty"`
	IPv6            bool   `json:"ipv6"`
	Rules           []Rule `json:"rules"`
}

func Collect(ctx context.Context) (Status, error) {
	bin, ok := find(ctx)
	if !ok {
		return Status{Rules: []Rule{}}, nil
	}
	verbose, err := remote.Run(ctx, bin, []string{"status", "verbose"}, "")
	if err != nil {
		return Status{}, fmt.Errorf("could not read the firewall: %s", short(err.Error()))
	}
	numbered, err := remote.Run(ctx, bin, []string{"status", "numbered"}, "")
	if err != nil {
		return Status{}, fmt.Errorf("could not read the firewall rules: %s", short(err.Error()))
	}
	status := Parse(verbose, numbered)
	status.Installed = true
	status.Logging = loggingLevel(status.Logging)
	status.IPv6 = true
	if raw, err := remote.ReadFile(ctx, "/etc/default/ufw"); err == nil {
		status.IPv6 = ipv6Enabled(string(raw))
	}
	if !status.Active {
		if added, err := remote.Run(ctx, bin, []string{"show", "added"}, ""); err == nil {
			if rules := ParseAdded(added); len(rules) > 0 {
				status.Rules = rules
			}
		}
		fillDefaults(ctx, &status)
		status.Logging = loggingLevel(status.Logging)
	}
	return status, nil
}

func AddRule(ctx context.Context, action, direction string, port int, proto, from, comment string, sshPort int) error {
	args, err := RuleArgs(action, direction, port, proto, from, comment, sshPort)
	if err != nil {
		return err
	}
	bin, ok := find(ctx)
	if !ok {
		return fmt.Errorf("install the firewall first")
	}
	if _, err := remote.Run(ctx, bin, args, ""); err != nil {
		return fmt.Errorf("could not add the firewall rule: %s", short(err.Error()))
	}
	return nil
}

func Delete(ctx context.Context, number, sshPort int) error {
	if number < 1 || sshPort < 1 || sshPort > 65535 {
		return fmt.Errorf("invalid firewall rule")
	}
	status, err := Collect(ctx)
	if err != nil {
		return err
	}
	var target *Rule
	for i := range status.Rules {
		if status.Rules[i].Number == number {
			target = &status.Rules[i]
			break
		}
	}
	if target == nil {
		return fmt.Errorf("firewall rule was not found")
	}
	if allowsPort(*target, sshPort) && !sshReachable(withoutRule(status.Rules, target.Number), sshPort) {
		return fmt.Errorf("SSH on port %d must stay allowed", sshPort)
	}
	bin, ok := find(ctx)
	if !ok {
		return fmt.Errorf("install the firewall first")
	}
	args := []string{"--force", "delete", strconv.Itoa(number)}
	if len(target.DeleteArgs) > 0 {
		args = append([]string{"delete"}, target.DeleteArgs...)
	}
	if _, err := remote.Run(ctx, bin, args, ""); err != nil {
		return fmt.Errorf("could not delete the firewall rule: %s", short(err.Error()))
	}
	return nil
}

func Update(ctx context.Context, number int, action, direction string, port int, proto, from, comment string, sshPort int) error {
	if number < 1 || sshPort < 1 || sshPort > 65535 {
		return fmt.Errorf("invalid firewall rule")
	}
	status, err := Collect(ctx)
	if err != nil {
		return err
	}
	target, ok := findRule(status.Rules, number)
	if !ok {
		return fmt.Errorf("firewall rule was not found")
	}
	if target.V6 {
		return fmt.Errorf("edit the IPv4 rule and the IPv6 rule follows it")
	}
	if _, _, ok := rulePort(target.To); !ok {
		return fmt.Errorf("this firewall rule cannot be edited")
	}
	if describes(target, action, direction, port, proto, from, comment) {
		return nil
	}
	args, err := RuleArgs(action, direction, port, proto, from, comment, sshPort)
	if err != nil {
		return err
	}
	preview := previewRule(action, direction, port, proto, from, comment)
	if mustKeepSSH(status) && !sshReachable(append(withoutMatching(status.Rules, target), preview), sshPort) {
		return fmt.Errorf("SSH on port %d must stay allowed", sshPort)
	}
	bin, ok := find(ctx)
	if !ok {
		return fmt.Errorf("install the firewall first")
	}
	cmd := args
	if status.Active {
		cmd = append([]string{"insert", strconv.Itoa(number)}, args...)
	}
	out, err := remote.Run(ctx, bin, cmd, "")
	if err != nil {
		return fmt.Errorf("could not update the firewall rule: %s", short(err.Error()))
	}
	if strings.Contains(strings.ToLower(out), "skipping") {
		return replaceSkipped(ctx, bin, target, args, sshPort)
	}
	return dropMatching(ctx, bin, target, sshPort)
}

func Enable(ctx context.Context, sshPort int) error {
	if sshPort < 1 || sshPort > 65535 {
		return fmt.Errorf("invalid ssh port")
	}
	bin, ok := find(ctx)
	if !ok {
		return fmt.Errorf("install the firewall first")
	}
	if err := ensureSSH(ctx, bin, sshPort); err != nil {
		return err
	}
	current, err := Collect(ctx)
	if err != nil {
		return err
	}
	if current.DefaultIncoming == "" {
		if _, err := remote.Run(ctx, bin, []string{"default", "deny", "incoming"}, ""); err != nil {
			return fmt.Errorf("could not set the firewall defaults: %s", short(err.Error()))
		}
	}
	if current.DefaultOutgoing == "" {
		if _, err := remote.Run(ctx, bin, []string{"default", "allow", "outgoing"}, ""); err != nil {
			return fmt.Errorf("could not set the firewall defaults: %s", short(err.Error()))
		}
	}
	if _, err := remote.Run(ctx, bin, []string{"--force", "enable"}, ""); err != nil {
		return fmt.Errorf("could not enable the firewall: %s", short(err.Error()))
	}
	checked, err := Collect(ctx)
	if err != nil || !sshReachable(checked.Rules, sshPort) {
		_, _ = remote.Run(ctx, bin, []string{"--force", "disable"}, "")
		return fmt.Errorf("SSH on port %d was not allowed, so the firewall was turned back off", sshPort)
	}
	return nil
}

func ensureSSH(ctx context.Context, bin string, sshPort int) error {
	status, err := Collect(ctx)
	if err != nil {
		return err
	}
	if sshReachable(status.Rules, sshPort) {
		return nil
	}
	if _, err := remote.Run(ctx, bin, []string{"insert", "1", "allow", strconv.Itoa(sshPort) + "/tcp", "comment", "SSH"}, ""); err != nil {
		return fmt.Errorf("could not allow SSH on port %d: %s", sshPort, short(err.Error()))
	}
	status, err = Collect(ctx)
	if err != nil {
		return err
	}
	if sshAllows(status.Rules, sshPort) == 0 {
		return fmt.Errorf("SSH on port %d could not be allowed, so the firewall was not enabled", sshPort)
	}
	return nil
}

func Disable(ctx context.Context) error {
	bin, ok := find(ctx)
	if !ok {
		return fmt.Errorf("install the firewall first")
	}
	if _, err := remote.Run(ctx, bin, []string{"--force", "disable"}, ""); err != nil {
		return fmt.Errorf("could not disable the firewall: %s", short(err.Error()))
	}
	return nil
}

func CollectJSON(ctx context.Context) (string, error) {
	status, err := Collect(ctx)
	if err != nil {
		return "", err
	}
	b, err := json.Marshal(status)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func AllowArgs(port int, proto, from, comment string) ([]string, error) {
	return RuleArgs("allow", "in", port, proto, from, comment, 0)
}

func RuleArgs(action, direction string, port int, proto, from, comment string, sshPort int) ([]string, error) {
	if err := validate.FirewallAccess(action, direction, port, proto, from, comment, sshPort); err != nil {
		return nil, err
	}
	args := []string{action}
	if direction == "out" {
		args = append(args, "out")
	}
	if from == "any" {
		if proto == "any" {
			args = append(args, strconv.Itoa(port))
		} else {
			args = append(args, strconv.Itoa(port)+"/"+proto)
		}
	} else {
		args = append(args, "from", from, "to", "any", "port", strconv.Itoa(port))
		if proto != "any" {
			args = append(args, "proto", proto)
		}
	}
	if comment != "" && comment != "-" {
		args = append(args, "comment", comment)
	}
	return args, nil
}

func ApplyConfig(ctx context.Context, incoming, outgoing, logging string, ipv6 bool, sshPort int) error {
	if err := validate.FirewallPolicy("incoming", incoming); err != nil {
		return err
	}
	if err := validate.FirewallPolicy("outgoing", outgoing); err != nil {
		return err
	}
	if err := validate.FirewallLogging(logging); err != nil {
		return err
	}
	bin, ok := find(ctx)
	if !ok {
		return fmt.Errorf("install the firewall first")
	}
	if incoming == "deny" || incoming == "reject" {
		if err := ensureSSH(ctx, bin, sshPort); err != nil {
			return err
		}
	}
	if _, err := remote.Run(ctx, bin, []string{"default", incoming, "incoming"}, ""); err != nil {
		return fmt.Errorf("could not set the incoming policy: %s", short(err.Error()))
	}
	if _, err := remote.Run(ctx, bin, []string{"default", outgoing, "outgoing"}, ""); err != nil {
		return fmt.Errorf("could not set the outgoing policy: %s", short(err.Error()))
	}
	if _, err := remote.Run(ctx, bin, []string{"logging", logging}, ""); err != nil {
		return fmt.Errorf("could not set firewall logging: %s", short(err.Error()))
	}
	changed, err := setIPv6(ctx, ipv6)
	if err != nil {
		return err
	}
	status, err := Collect(ctx)
	if err != nil {
		return err
	}
	if !status.Active || !changed {
		return nil
	}
	if incoming == "deny" || incoming == "reject" {
		if err := ensureSSH(ctx, bin, sshPort); err != nil {
			return err
		}
	}
	if _, err := remote.Run(ctx, bin, []string{"reload"}, ""); err != nil {
		return fmt.Errorf("could not reload the firewall: %s", short(err.Error()))
	}
	if incoming != "deny" && incoming != "reject" {
		return nil
	}
	checked, err := Collect(ctx)
	if err != nil || !sshReachable(checked.Rules, sshPort) {
		_, _ = remote.Run(ctx, bin, []string{"--force", "disable"}, "")
		return fmt.Errorf("SSH on port %d was not allowed, so the firewall was turned back off", sshPort)
	}
	return nil
}

func Parse(verbose, numbered string) Status {
	status := Status{Rules: []Rule{}}
	for _, line := range strings.Split(verbose, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "Status:"):
			status.Active = strings.Contains(strings.ToLower(line), "active") && !strings.Contains(strings.ToLower(line), "inactive")
		case strings.HasPrefix(line, "Logging:"):
			status.Logging = strings.TrimSpace(strings.TrimPrefix(line, "Logging:"))
		case strings.HasPrefix(line, "Default:"):
			if m := defaultRe.FindStringSubmatch(line); m != nil {
				status.DefaultIncoming = strings.ToLower(m[1])
				status.DefaultOutgoing = strings.ToLower(m[2])
			}
		}
	}
	for _, line := range strings.Split(numbered, "\n") {
		if rule, ok := parseRule(line); ok {
			status.Rules = append(status.Rules, rule)
		}
	}
	return status
}

func sshAllows(rules []Rule, port int) int {
	n := 0
	for _, rule := range rules {
		if allowsPort(rule, port) {
			n++
		}
	}
	return n
}

func withoutRule(rules []Rule, number int) []Rule {
	rest := make([]Rule, 0, len(rules))
	for _, rule := range rules {
		if rule.Number != number {
			rest = append(rest, rule)
		}
	}
	return rest
}

func sshReachable(rules []Rule, port int) bool {
	for _, rule := range rules {
		if rule.V6 {
			continue
		}
		if allowsPort(rule, port) {
			return true
		}
		if blocksSSH(rule, port) {
			return false
		}
	}
	return false
}

func allowsPort(rule Rule, port int) bool {
	if rule.V6 || !fromAnywhere(rule.From) {
		return false
	}
	action := strings.ToUpper(rule.Action)
	if !strings.HasPrefix(action, "ALLOW") || strings.Contains(action, "OUT") {
		return false
	}
	got, proto, ok := rulePort(rule.To)
	return ok && got == port && (proto == "tcp" || proto == "any")
}

func blocksSSH(rule Rule, port int) bool {
	action := strings.ToUpper(rule.Action)
	if strings.Contains(action, "OUT") || !fromAnywhere(rule.From) {
		return false
	}
	if !strings.HasPrefix(action, "DENY") && !strings.HasPrefix(action, "REJECT") {
		return false
	}
	if strings.EqualFold(rule.To, "Anywhere") || strings.EqualFold(rule.To, "any") {
		return true
	}
	got, proto, ok := rulePort(rule.To)
	return ok && got == port && (proto == "tcp" || proto == "any")
}

func fromAnywhere(from string) bool {
	switch strings.ToLower(strings.TrimSpace(from)) {
	case "", "any", "anywhere", "0.0.0.0/0", "0.0.0.0":
		return true
	default:
		return false
	}
}

func findRule(rules []Rule, number int) (Rule, bool) {
	for _, rule := range rules {
		if rule.Number == number {
			return rule, true
		}
	}
	return Rule{}, false
}

func mustKeepSSH(status Status) bool {
	if status.Active {
		return true
	}
	switch status.DefaultIncoming {
	case "", "allow":
		return false
	default:
		return true
	}
}

func withoutMatching(rules []Rule, old Rule) []Rule {
	rest := make([]Rule, 0, len(rules))
	for _, rule := range rules {
		if rule.Number == old.Number || sameDisplayed(rule, old) {
			continue
		}
		rest = append(rest, rule)
	}
	return rest
}

func sameDisplayed(a, b Rule) bool {
	return a.To == b.To && strings.EqualFold(a.Action, b.Action) && strings.EqualFold(a.From, b.From) && a.Comment == b.Comment
}

func describes(rule Rule, action, direction string, port int, proto, from, comment string) bool {
	if rule.V6 || !strings.EqualFold(rule.Action, actionLabel(action, direction)) || rule.Comment != comment {
		return false
	}
	got, gotProto, ok := rulePort(rule.To)
	if !ok || got != port || gotProto != proto {
		return false
	}
	if from == "" || from == "any" {
		return fromAnywhere(rule.From)
	}
	return strings.EqualFold(rule.From, from)
}

func previewRule(action, direction string, port int, proto, from, comment string) Rule {
	to := strconv.Itoa(port)
	if proto != "any" {
		to += "/" + proto
	}
	src := "Anywhere"
	if from != "" && from != "any" {
		src = from
	}
	return Rule{To: to, Action: actionLabel(action, direction), From: src, Comment: comment}
}

func actionLabel(action, direction string) string {
	label := strings.ToUpper(action)
	if direction == "out" {
		return label + " OUT"
	}
	return label + " IN"
}

func replaceSkipped(ctx context.Context, bin string, old Rule, args []string, sshPort int) error {
	if err := deleteRule(ctx, bin, old, sshPort); err != nil {
		return err
	}
	if _, err := remote.Run(ctx, bin, args, ""); err != nil {
		if restore, restoreErr := commandFor(old); restoreErr == nil {
			_, _ = remote.Run(ctx, bin, restore, "")
		}
		return fmt.Errorf("could not update the firewall rule: %s", short(err.Error()))
	}
	return nil
}

func dropMatching(ctx context.Context, bin string, old Rule, sshPort int) error {
	for n := 0; n < 8; n++ {
		status, err := Collect(ctx)
		if err != nil {
			return err
		}
		var victim *Rule
		for i := range status.Rules {
			if sameDisplayed(status.Rules[i], old) {
				victim = &status.Rules[i]
			}
		}
		if victim == nil {
			return nil
		}
		if err := deleteRule(ctx, bin, *victim, sshPort); err != nil {
			return err
		}
	}
	return nil
}

func deleteRule(ctx context.Context, bin string, rule Rule, sshPort int) error {
	status, err := Collect(ctx)
	if err != nil {
		return err
	}
	current, ok := findRule(status.Rules, rule.Number)
	if !ok {
		for i := range status.Rules {
			if sameDisplayed(status.Rules[i], rule) {
				current = status.Rules[i]
				ok = true
			}
		}
	}
	if !ok {
		return nil
	}
	if allowsPort(current, sshPort) && !sshReachable(withoutRule(status.Rules, current.Number), sshPort) {
		return fmt.Errorf("SSH on port %d must stay allowed", sshPort)
	}
	args := []string{"--force", "delete", strconv.Itoa(current.Number)}
	if len(current.DeleteArgs) > 0 {
		args = append([]string{"delete"}, current.DeleteArgs...)
	}
	if _, err := remote.Run(ctx, bin, args, ""); err != nil {
		return fmt.Errorf("could not remove the previous firewall rule: %s", short(err.Error()))
	}
	return nil
}

func commandFor(rule Rule) ([]string, error) {
	fields := strings.Fields(rule.Action)
	if len(fields) == 0 {
		return nil, fmt.Errorf("this firewall rule cannot be edited")
	}
	action := strings.ToLower(fields[0])
	direction := "in"
	if strings.Contains(strings.ToUpper(rule.Action), "OUT") {
		direction = "out"
	}
	port, proto, ok := rulePort(rule.To)
	if !ok {
		return nil, fmt.Errorf("this firewall rule cannot be edited")
	}
	from := "any"
	if !fromAnywhere(rule.From) {
		from = rule.From
	}
	return RuleArgs(action, direction, port, proto, from, rule.Comment, 0)
}

func rulePort(to string) (int, string, bool) {
	to = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(to), "(v6)"))
	portPart, proto, ok := strings.Cut(to, "/")
	if !ok {
		portPart = to
		proto = "any"
	}
	n, err := strconv.Atoi(portPart)
	if err != nil || n < 1 || n > 65535 {
		return 0, "", false
	}
	return n, proto, true
}

var (
	defaultRe = regexp.MustCompile(`(?i)default:\s*([a-z]+)\s*\(incoming\),\s*([a-z]+)\s*\(outgoing\)`)
	ruleRe    = regexp.MustCompile(`^\[\s*(\d+)\]\s+(\S.*?)\s+(ALLOW|DENY|REJECT|LIMIT)(?:\s+(IN|OUT))?\s+(\S.*?)(?:\s+#\s*(.*))?$`)
)

func parseRule(line string) (Rule, bool) {
	m := ruleRe.FindStringSubmatch(strings.TrimSpace(line))
	if m == nil {
		return Rule{}, false
	}
	number, _ := strconv.Atoi(m[1])
	to := strings.TrimSpace(m[2])
	from := strings.TrimSpace(m[5])
	v6 := strings.Contains(to, "(v6)") || strings.Contains(from, "(v6)")
	to = strings.TrimSpace(strings.TrimSuffix(to, "(v6)"))
	from = strings.TrimSpace(strings.TrimSuffix(from, "(v6)"))
	comment := strings.TrimSpace(m[6])
	if comment == "" {
		if cut, rest, ok := strings.Cut(from, "#"); ok {
			from = strings.TrimSpace(cut)
			comment = strings.TrimSpace(rest)
		}
	}
	action := m[3]
	if m[4] != "" {
		action += " " + m[4]
	}
	return Rule{Number: number, To: to, Action: action, From: from, Comment: comment, V6: v6}, true
}

func ParseAdded(text string) []Rule {
	var rules []Rule
	for _, line := range strings.Split(text, "\n") {
		fields := splitQuoted(strings.TrimSpace(line))
		if len(fields) < 2 || fields[0] != "ufw" {
			continue
		}
		rule, ok := addedRule(fields[1:])
		if !ok {
			continue
		}
		rule.Number = len(rules) + 1
		rules = append(rules, rule)
	}
	return rules
}

func addedRule(args []string) (Rule, bool) {
	if len(args) >= 3 && strings.EqualFold(args[0], "insert") {
		if _, err := strconv.Atoi(args[1]); err == nil {
			args = args[2:]
		}
	}
	action := strings.ToUpper(args[0])
	switch action {
	case "ALLOW", "DENY", "REJECT", "LIMIT":
	default:
		return Rule{}, false
	}
	rest := append([]string{}, args[1:]...)
	comment := ""
	if len(rest) >= 2 && strings.EqualFold(rest[len(rest)-2], "comment") {
		comment = rest[len(rest)-1]
		rest = rest[:len(rest)-2]
	}
	direction := "IN"
	if len(rest) > 0 && (strings.EqualFold(rest[0], "in") || strings.EqualFold(rest[0], "out")) {
		direction = strings.ToUpper(rest[0])
		rest = rest[1:]
	}
	if len(rest) == 0 {
		return Rule{}, false
	}
	rule := Rule{Action: action + " " + direction, From: "Anywhere", Comment: comment, DeleteArgs: append([]string{}, args...)}
	if rest[0] != "from" {
		rule.To = rest[0]
		return rule, true
	}
	if len(rest) < 2 {
		return Rule{}, false
	}
	rule.From = rest[1]
	port := ""
	proto := "any"
	for i := 2; i < len(rest); i++ {
		switch rest[i] {
		case "port":
			if i+1 < len(rest) {
				port = rest[i+1]
				i++
			}
		case "proto":
			if i+1 < len(rest) {
				proto = rest[i+1]
				i++
			}
		}
	}
	if port == "" {
		rule.To = "Anywhere"
	} else if proto == "any" {
		rule.To = port
	} else {
		rule.To = port + "/" + proto
	}
	return rule, true
}

func splitQuoted(line string) []string {
	var out []string
	var b strings.Builder
	var quote rune
	for _, r := range line {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				b.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote = r
		case r == ' ' || r == '\t':
			if b.Len() > 0 {
				out = append(out, b.String())
				b.Reset()
			}
		default:
			b.WriteRune(r)
		}
	}
	if b.Len() > 0 {
		out = append(out, b.String())
	}
	return out
}

func fillDefaults(ctx context.Context, status *Status) {
	if status.DefaultIncoming == "" || status.DefaultOutgoing == "" {
		if raw, err := remote.ReadFile(ctx, "/etc/default/ufw"); err == nil {
			in, out := policies(string(raw))
			if status.DefaultIncoming == "" {
				status.DefaultIncoming = in
			}
			if status.DefaultOutgoing == "" {
				status.DefaultOutgoing = out
			}
		}
	}
	if status.Logging == "" {
		if raw, err := remote.ReadFile(ctx, "/etc/ufw/ufw.conf"); err == nil {
			status.Logging = logLevel(string(raw))
		}
	}
}

func policies(text string) (string, string) {
	in, out := "", ""
	for _, line := range strings.Split(text, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		switch key {
		case "DEFAULT_INPUT_POLICY":
			in = policyName(value)
		case "DEFAULT_OUTPUT_POLICY":
			out = policyName(value)
		}
	}
	return in, out
}

func policyName(value string) string {
	switch strings.ToUpper(value) {
	case "DROP", "DENY":
		return "deny"
	case "ACCEPT", "ALLOW":
		return "allow"
	case "REJECT":
		return "reject"
	default:
		return strings.ToLower(value)
	}
}

func logLevel(text string) string {
	for _, line := range strings.Split(text, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if ok && key == "LOGLEVEL" {
			return strings.Trim(strings.TrimSpace(value), `"'`)
		}
	}
	return ""
}

func loggingLevel(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	switch {
	case raw == "" || raw == "off":
		return "off"
	case strings.Contains(raw, "full"):
		return "full"
	case strings.Contains(raw, "high"):
		return "high"
	case strings.Contains(raw, "medium"):
		return "medium"
	case strings.Contains(raw, "low"), strings.Contains(raw, "on"):
		return "low"
	default:
		return "off"
	}
}

func ipv6Enabled(text string) bool {
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		key, value, ok := strings.Cut(trimmed, "=")
		if !ok || strings.TrimSpace(key) != "IPV6" {
			continue
		}
		value = strings.ToLower(strings.Trim(strings.TrimSpace(value), `"'`))
		return value == "yes" || value == "true" || value == "1"
	}
	return true
}

func setIPv6(ctx context.Context, on bool) (bool, error) {
	raw, err := remote.ReadFile(ctx, "/etc/default/ufw")
	if err != nil {
		return false, fmt.Errorf("could not read the firewall configuration")
	}
	value := "no"
	if on {
		value = "yes"
	}
	next := setConfigValue(string(raw), "IPV6", value)
	if next == string(raw) {
		return false, nil
	}
	if err := remote.WriteFile(ctx, "/etc/default/ufw", []byte(next), 0o644); err != nil {
		return false, fmt.Errorf("could not save the firewall configuration")
	}
	return true, nil
}

func setConfigValue(text, key, value string) string {
	lines := strings.Split(text, "\n")
	found := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		name, _, ok := strings.Cut(trimmed, "=")
		if ok && strings.TrimSpace(name) == key {
			lines[i] = key + "=" + value
			found = true
		}
	}
	if found {
		return strings.Join(lines, "\n")
	}
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	return text + key + "=" + value + "\n"
}

func find(ctx context.Context) (string, bool) {
	for _, path := range []string{"/usr/sbin/ufw", "/usr/bin/ufw"} {
		if _, err := remote.Stat(ctx, path); err == nil {
			return path, true
		}
	}
	return "", false
}

func short(msg string) string {
	msg = strings.Join(strings.Fields(msg), " ")
	if len(msg) > 240 {
		msg = msg[:240]
	}
	if msg == "" {
		return "firewall command failed"
	}
	return msg
}
