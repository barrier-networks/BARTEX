package db

import "fmt"

// builtinTaskTemplate is one shipped starter template. nkey is the stable unique
// key (ON CONFLICT target); name/description/goal are what the operator sees and
// what seeds a new task when the template is applied.
type builtinTaskTemplate struct {
	nkey        string
	name        string
	description string
	goal        string
}

// builtinTaskTemplates are the curated starting points seeded on first startup.
// The goals are written to steer the planner toward the matching skill
// (cloud-recon, external-infra, …); operators edit or delete them freely.
var builtinTaskTemplates = []builtinTaskTemplate{
	{
		nkey:        "aws-cloud-pacu",
		name:        "AWS Cloud Assessment (Pacu)",
		description: "Authorized, read-only AWS security assessment: external exposure plus a credentialed posture review driven headlessly with Pacu (read-only enumeration and IAM privilege-escalation path analysis).",
		goal: "Assess the authorized AWS account(s) named in this task from a strictly read-only posture.\n" +
			"1) External / anonymous: enumerate publicly exposed resources for the in-scope account IDs — S3 buckets, public services, exposed endpoints — and confirm each exposure anonymously before reporting it.\n" +
			"2) Credentialed read: using the operator-supplied scoped read-only keys (task credential, domain=aws), run a CSPM-style posture review across IAM, network, logging, encryption and public-access settings, and drive Pacu headlessly for read-only enumeration (whoami / enum modules) and IAM privilege-escalation PATH ANALYSIS only (iam__privesc_scan — no changes).\n" +
			"Read-only always: never Put/Create/Delete/Update/Attach, never AssumeRole into another account, never exfiltrate data, and enumerate only within the named account IDs. Report confirmed public exposure and high-risk misconfigurations with evidence.",
	},
	{
		nkey:        "external-infra",
		name:        "External Infrastructure",
		description: "Perimeter and network assessment of the in-scope domains / IP ranges: host and service discovery, exposed services, and version/misconfiguration surface.",
		goal: "Map and assess the external perimeter of the in-scope domains and IP ranges.\n" +
			"Discover live hosts and services, fingerprint exposed services and their versions, and identify misconfigurations, default credentials and known-vulnerable services. Stay within the declared scope; confirm each finding with evidence before reporting.",
	},
	{
		nkey:        "web-app",
		name:        "Web Application Assessment",
		description: "Authenticated and unauthenticated testing of the in-scope web application(s): the common web risk classes with evidence-backed findings.",
		goal: "Assess the in-scope web application(s).\n" +
			"Enumerate the surface (routes, parameters, auth flows), then test for the common risk classes — injection, broken access control / IDOR, authentication and session flaws, SSRF, and misconfiguration. Use any operator-supplied test accounts (task credentials). Confirm each issue with a concrete, reproducible proof before reporting.",
	},
	{
		nkey:        "attack-surface-bbot",
		name:        "Attack Surface Discovery (bbot)",
		description: "DNS and subdomain breadth for the in-scope root domain(s): enumerate the external attack surface and feed confirmed in-scope hosts into the asset graph.",
		goal: "Enumerate the external attack surface of the in-scope root domain(s).\n" +
			"Use passive and active DNS/subdomain discovery (bbot) to find subdomains, related hosts and exposed services; keep only in-scope results and record them as assets. Hand confirmed live services off for deeper assessment. Breadth first — do not exploit here.",
	},
}

// SeedBuiltinTaskTemplates inserts the curated starter templates once, on first
// startup. It is gated by a settings flag so a user's later edits or deletes are
// never resurrected on restart (same policy as the built-in intercept rules and
// skill visibility). ON CONFLICT (nkey) DO NOTHING additionally protects an
// operator who created a template under one of these keys first.
func (d *DB) SeedBuiltinTaskTemplates() error {
	if v, _, _ := d.GetSetting("task_templates_seed_v1"); v == "done" {
		return nil
	}
	for _, t := range builtinTaskTemplates {
		if _, err := d.Exec(`
INSERT INTO task_templates(name, nkey, description, goal, intercept_rules)
VALUES ($1, $2, $3, $4, '[]'::jsonb)
ON CONFLICT (nkey) DO NOTHING`, t.name, t.nkey, t.description, t.goal); err != nil {
			return fmt.Errorf("seed task template %q: %w", t.nkey, err)
		}
	}
	return d.SetSetting("task_templates_seed_v1", "done")
}
