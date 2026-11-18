package report

import "html/template"

// clientReportTmpl is the Barrier-branded, print-friendly client report. It is a
// standalone HTML document (light theme for print/PDF), structured along UK
// penetration-test reporting convention.
var clientReportTmpl = template.Must(template.New("clientReport").Parse(clientReportHTML))

const clientReportHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Meta.Client}} — {{.Meta.Title}}</title>
<style>
  :root {
    --brand: #1e6feb; --accent: #0e9f8e; --ink: #0b1220; --muted: #51607a;
    --line: #d6deea; --bg: #ffffff; --soft: #f5f7fa;
    --sev-critical: #b91c1c; --sev-high: #ea580c; --sev-medium: #b45309;
    --sev-low: #2563eb; --sev-info: #6b7280;
  }
  * { box-sizing: border-box; }
  body { margin: 0; color: var(--ink); background: var(--bg);
    font-family: "Inter", ui-sans-serif, system-ui, -apple-system, Segoe UI, Roboto, Arial, sans-serif;
    font-size: 11pt; line-height: 1.5; }
  code, pre, .mono { font-family: "JetBrains Mono", ui-monospace, SFMono-Regular, Menlo, monospace; }
  .page { max-width: 900px; margin: 0 auto; padding: 32px 40px; }
  h1, h2, h3 { color: var(--ink); line-height: 1.25; }
  h2 { border-bottom: 2px solid var(--brand); padding-bottom: 6px; margin-top: 40px; }
  h3 { margin-top: 28px; }
  a { color: var(--brand); }
  .classification { text-align: center; letter-spacing: 3px; font-weight: 700;
    color: var(--sev-critical); border: 1px solid var(--sev-critical);
    padding: 6px; border-radius: 4px; text-transform: uppercase; }
  .cover { min-height: 70vh; display: flex; flex-direction: column; justify-content: center; }
  .brand { display: flex; align-items: center; gap: 12px; margin-bottom: 24px; }
  .brand .mark { width: 40px; height: 40px; }
  .brand .name { font-size: 20pt; font-weight: 800; letter-spacing: 1px; }
  .brand .name .net { color: var(--brand); }
  .cover h1 { font-size: 30pt; margin: 8px 0 4px; }
  .cover .client { font-size: 16pt; color: var(--brand); font-weight: 700; }
  .cover dl { margin-top: 28px; display: grid; grid-template-columns: max-content 1fr; gap: 6px 18px; }
  .cover dt { color: var(--muted); font-weight: 600; }
  table { width: 100%; border-collapse: collapse; margin: 14px 0; }
  th, td { text-align: left; padding: 8px 10px; border-bottom: 1px solid var(--line); vertical-align: top; }
  th { background: var(--soft); font-size: 9.5pt; text-transform: uppercase; letter-spacing: .5px; color: var(--muted); }
  .pill { display: inline-block; padding: 2px 9px; border-radius: 999px; color: #fff;
    font-size: 9pt; font-weight: 700; text-transform: uppercase; letter-spacing: .4px; }
  .sev-critical { background: var(--sev-critical); }
  .sev-high { background: var(--sev-high); }
  .sev-medium { background: var(--sev-medium); }
  .sev-low { background: var(--sev-low); }
  .sev-info { background: var(--sev-info); }
  .score { font-weight: 700; }
  .summary-cards { display: flex; gap: 10px; flex-wrap: wrap; margin: 16px 0; }
  .card { flex: 1 1 110px; border: 1px solid var(--line); border-radius: 8px; padding: 12px 14px; }
  .card .n { font-size: 22pt; font-weight: 800; }
  .card .l { font-size: 9pt; text-transform: uppercase; letter-spacing: .5px; color: var(--muted); }
  .finding { border: 1px solid var(--line); border-radius: 8px; padding: 0 18px 14px; margin: 18px 0; }
  .finding > .head { display: flex; align-items: center; gap: 10px; margin: 0 -18px 10px;
    padding: 12px 18px; border-bottom: 1px solid var(--line); background: var(--soft); border-radius: 8px 8px 0 0; }
  .finding .head .idx { color: var(--muted); font-weight: 700; }
  .finding .head .title { font-weight: 700; font-size: 12.5pt; flex: 1; }
  .meta-row { display: flex; flex-wrap: wrap; gap: 8px 20px; margin: 10px 0; font-size: 10pt; color: var(--muted); }
  .meta-row b { color: var(--ink); }
  pre.evidence { background: var(--soft); border: 1px solid var(--line); border-radius: 6px;
    padding: 10px 12px; overflow-x: auto; font-size: 9.5pt; white-space: pre-wrap; word-break: break-word; }
  .muted { color: var(--muted); }
  .indicative { font-style: italic; color: var(--muted); font-size: 9pt; }
  footer { margin-top: 40px; border-top: 1px solid var(--line); padding-top: 12px;
    color: var(--muted); font-size: 9pt; display: flex; justify-content: space-between; }
  @media print {
    .page { max-width: none; padding: 0; }
    h2 { break-after: avoid; } h3 { break-after: avoid; }
    .finding { break-inside: avoid; }
    .cover { min-height: 90vh; }
  }
</style>
</head>
<body>
<div class="page">

  <section class="cover">
    <div class="classification">{{.Meta.Classification}}</div>
    <div class="brand" style="margin-top:40px">
      <svg class="mark" viewBox="0 0 64 64" xmlns="http://www.w3.org/2000/svg" aria-hidden="true">
        <path fill="#1E6FEB" fill-rule="evenodd" clip-rule="evenodd"
          d="M32 3 L57 13 L57 31 C57 47 46 57.5 32 61.5 C18 57.5 7 47 7 31 L7 13 Z
             M20 40 H25 V48 H20 Z M29.5 32 H34.5 V48 H29.5 Z M39 24 H44 V48 H39 Z"/>
      </svg>
      <div class="name">BARRIER<span class="net"> NETWORKS</span></div>
    </div>
    <h1>{{.Meta.Title}}</h1>
    <div class="client">{{.Meta.Client}}</div>
    <dl>
      {{if .Meta.EngagementName}}<dt>Engagement</dt><dd>{{.Meta.EngagementName}}</dd>{{end}}
      {{if or .Meta.PeriodStart .Meta.PeriodEnd}}<dt>Testing window</dt><dd>{{.Meta.PeriodStart}}{{if .Meta.PeriodEnd}} &ndash; {{.Meta.PeriodEnd}}{{end}}</dd>{{end}}
      {{if .Meta.ScopeSummary}}<dt>Authorised scope</dt><dd>{{.Meta.ScopeSummary}}</dd>{{end}}
      {{if .Meta.Reference}}<dt>Authorisation ref.</dt><dd>{{.Meta.Reference}}</dd>{{end}}
      <dt>Prepared by</dt><dd>{{.Meta.Author}}</dd>
      <dt>Generated</dt><dd>{{.GeneratedAt}}</dd>
    </dl>
  </section>

  <h2>Document control</h2>
  <table>
    <tr><th>Field</th><th>Value</th></tr>
    <tr><td>Classification</td><td>{{.Meta.Classification}}</td></tr>
    <tr><td>Client</td><td>{{.Meta.Client}}</td></tr>
    {{if .Meta.EngagementName}}<tr><td>Engagement</td><td>{{.Meta.EngagementName}}</td></tr>{{end}}
    {{if .Meta.Reference}}<tr><td>Authorisation reference</td><td>{{.Meta.Reference}}</td></tr>{{end}}
    <tr><td>Prepared by</td><td>{{.Meta.Author}}</td></tr>
    <tr><td>Report generated</td><td>{{.GeneratedAt}}</td></tr>
  </table>

  <h2>Executive summary</h2>
  <p>This report presents the results of an authorised penetration test carried out by
    {{.Meta.Author}} on behalf of {{.Meta.Client}}. The assessment identified
    <b>{{.Total}}</b> finding{{if ne .Total 1}}s{{end}} across the authorised scope.
    {{if eq .HighestSeverity "critical"}}Critical issues were identified that require urgent remediation.
    {{else if eq .HighestSeverity "high"}}High-severity issues were identified that should be remediated promptly.
    {{else if eq .HighestSeverity "medium"}}The issues identified are of moderate severity and should be scheduled for remediation.
    {{else if eq .HighestSeverity "low"}}Only low-severity issues were identified; the overall exposure is limited.
    {{else if eq .HighestSeverity "info"}}Only informational observations were recorded.
    {{else}}No findings were recorded within the authorised scope for this assessment.{{end}}
    Each finding below carries the evidence captured during testing and a CVSS v3.1 score to support prioritisation.</p>

  <div class="summary-cards">
    {{range .SeverityCounts}}<div class="card"><div class="n">{{.Count}}</div><div class="l"><span class="pill sev-{{.Class}}">{{.Severity}}</span></div></div>{{end}}
  </div>

  <h2>Scope &amp; methodology</h2>
  <p>Testing was conducted only against the authorised scope{{if .Meta.ScopeSummary}} ({{.Meta.ScopeSummary}}){{end}}
    and governed throughout by the platform's scope, approval and rules-of-engagement controls: every action is
    bound to the engagement's authorised targets and testing window, and autonomous actions pass through
    human-in-the-loop approval gates. The methodology aligns with industry guidance including the
    <b>OWASP Testing Guide</b>, <b>NIST SP 800-115</b> and, where applicable, the <b>NCSC CAF</b>.
    Findings are triaged and, for access-control issues, proven against a second account or role before being reported.</p>

  <h2>Findings summary</h2>
  {{if .Findings}}
  <table>
    <tr><th>#</th><th>Finding</th><th>Severity</th><th>CVSS</th><th>Status</th></tr>
    {{range .Findings}}
    <tr>
      <td>{{.Index}}</td>
      <td>{{.Title}}</td>
      <td><span class="pill sev-{{.SeverityClass}}">{{.Severity}}</span></td>
      <td class="score">{{.CVSSScore}}{{if .CVSSIndicative}} <span class="indicative">(indicative)</span>{{end}}</td>
      <td>{{.Status}}</td>
    </tr>
    {{end}}
  </table>
  {{else}}<p class="muted">No findings were recorded.</p>{{end}}

  {{if .Findings}}
  <h2>Detailed findings</h2>
  {{range .Findings}}
  <div class="finding">
    <div class="head">
      <span class="idx">{{.Index}}</span>
      <span class="title">{{.Title}}</span>
      <span class="pill sev-{{.SeverityClass}}">{{.Severity}}</span>
    </div>
    <div class="meta-row">
      <span><b>CVSS v3.1:</b> {{.CVSSScore}} ({{.CVSSSeverity}}){{if .CVSSIndicative}} <span class="indicative">indicative, derived from severity</span>{{end}}</span>
      {{if .CVSSVector}}<span class="mono">{{.CVSSVector}}</span>{{end}}
      <span><b>Status:</b> {{.Status}}</span>
      <span><b>Verification:</b> {{.Verification}}</span>
      {{if gt .PoE 0}}<span><b>Proof-of-exploitation:</b> level {{.PoE}}</span>{{end}}
    </div>
    {{if .Summary}}<p>{{.Summary}}</p>{{end}}
    {{if .Evidence}}<h4>Evidence</h4><pre class="evidence">{{.Evidence}}</pre>{{end}}
    {{if .Traffic}}
    <h4>Captured traffic</h4>
    <table>
      <tr><th>Role</th><th>Method</th><th>URL</th><th>Status</th></tr>
      {{range .Traffic}}<tr><td>{{.Role}}</td><td>{{.Method}}</td><td class="mono">{{.URL}}</td><td>{{.Status}}</td></tr>{{end}}
    </table>
    {{end}}
    {{if .Counterevidence}}<h4>Counter-evidence (what did not work)</h4><p class="muted">{{.Counterevidence}}</p>{{end}}
    {{if .Confidence}}<h4>Confidence rationale</h4><p class="muted">{{.Confidence}}</p>{{end}}
    {{if .VerificationNote}}<p class="muted"><b>Verification note:</b> {{.VerificationNote}}</p>{{end}}
  </div>
  {{end}}
  {{end}}

  <h2>Appendix: scoring</h2>
  <p class="muted">Scores use the Common Vulnerability Scoring System (CVSS) v3.1 base metrics. Where a finding
    carries an analyst-assigned vector, its base score is computed from that vector. Where no vector has been
    assigned, an <i>indicative</i> score is derived from the finding's severity band to aid prioritisation and is
    labelled accordingly; it should be confirmed with a full vector before external publication.</p>

  <footer>
    <span>{{.Meta.Classification}} &middot; {{.Meta.Client}}</span>
    <span>Prepared by {{.Meta.Author}} &middot; {{.GeneratedAt}}</span>
  </footer>

</div>
</body>
</html>`
