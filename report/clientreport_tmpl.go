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
    /* Barrier brand (from the brand guidelines): Cobalt + Deep, Glow accent. */
    --brand: #0a45a3; --accent: #38baf5; --ink: #03122b; --muted: #596a83;
    --line: #e9ecf0; --bg: #ffffff; --soft: #f5f6f8;
    /* Status hues are published-fixed: Brick / Amber / Jade. */
    --sev-critical: #a73030; --sev-high: #b8510a; --sev-medium: #855400;
    --sev-low: #0a45a3; --sev-info: #596a83;
  }
  * { box-sizing: border-box; }
  body { margin: 0; color: var(--ink); background: var(--bg);
    font-family: "Aptos", "Calibri", "Segoe UI", system-ui, Arial, sans-serif;
    font-size: 11pt; line-height: 1.5; }
  code, pre, .mono { font-family: ui-monospace, "Cascadia Code", "Consolas", "Courier New", monospace; }
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
  .brandbar { align-self: flex-start; background: var(--ink); padding: 15px 22px; border-radius: 10px; }
  .brandbar svg { height: 34px; width: auto; display: block; }
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
    <div class="brandbar" style="margin-top:40px">
      <svg viewBox="0 0 320 76.2728" role="img" aria-label="Barrier Networks" fill="none" xmlns="http://www.w3.org/2000/svg">
        <path d="M70.8906 46.8841H85.1472L80.3495 32.8744C79.7409 31.0841 78.8458 28.6235 78.0211 25.8145C77.2671 28.6587 76.4072 31.1502 75.6928 32.945L70.8906 46.8841ZM104.418 66.4456H95.0338C92.9216 66.4456 91.559 65.3917 91.021 63.8836L87.943 54.9627H68.0993L65.0213 63.8836C64.5539 65.1844 63.0854 66.4456 61.079 66.4456H51.6246L71.8255 15.2753H84.2168L104.418 66.4456Z" fill="#fff"/>
        <path d="M122.678 39.3347H127.551C134.139 39.3347 136.719 35.8245 136.719 31.3266C136.719 26.8331 133.817 24.1608 127.551 24.1608H122.678V39.3347ZM122.678 47.5191V66.4456H110.574V15.2753H127.551C142.663 15.2753 148.537 21.7399 148.537 30.4491C148.537 37.0152 144.815 42.4964 138.046 45.0232C139.656 45.8302 141.124 46.9547 142.165 48.6392L153.264 66.4456H142.306C140.264 66.4456 138.831 65.7092 137.936 64.1966L129.271 49.3447C128.517 48.0439 127.622 47.5191 125.756 47.5191H122.678Z" fill="#fff"/>
        <path d="M170.607 39.3347H175.48C182.068 39.3347 184.648 35.8245 184.648 31.3266C184.648 26.8331 181.746 24.1608 175.48 24.1608H170.607V39.3347ZM170.607 47.5191V66.4456H158.503V15.2753H175.48C190.592 15.2753 196.466 21.7399 196.466 30.4491C196.466 37.0152 192.74 42.4964 185.975 45.0232C187.585 45.8302 189.053 46.9547 190.094 48.6392L201.193 66.4456H190.235C188.193 66.4456 186.76 65.7092 185.865 64.1966L177.196 49.3447C176.446 48.0439 175.551 47.5191 173.685 47.5191H170.607Z" fill="#fff"/>
        <path d="M219.683 66.4456H207.504V15.2797H219.683V66.4456Z" fill="#fff"/>
        <path d="M261.606 15.2753V24.4078H240.188V36.3493H256.593V45.129H240.188V57.3175H261.606V66.4456H228.009V15.2753H261.606Z" fill="#fff"/>
        <path d="M280.881 39.3347H285.75C292.342 39.3347 294.922 35.8245 294.922 31.3266C294.922 26.8331 292.02 24.1608 285.75 24.1608H280.881V39.3347ZM280.881 47.5191V66.4456H268.772V15.2753H285.75C300.866 15.2753 306.74 21.7399 306.74 30.4491C306.74 37.0152 303.014 42.4964 296.245 45.0232C297.859 45.8302 299.327 46.9547 300.363 48.6392L311.467 66.4456H300.509C298.467 66.4456 297.034 65.7092 296.139 64.1966L287.469 49.3447C286.715 48.0439 285.82 47.5191 283.959 47.5191H280.881Z" fill="#fff"/>
        <path d="M10.5392 38.8848C10.5392 38.8848 10.2658 66.3397 10.2658 66.9394C10.2658 67.5436 10.6892 67.9404 11.2668 67.9404H34.1797C41.4161 67.9404 50.5574 63.7777 50.5574 52.3124C50.5574 40.8427 41.8791 38.2234 41.1118 38.2234C40.3489 38.2234 40.3489 37.4561 41.1118 37.4561C41.8791 37.4561 47.034 34.2414 47.034 26.2289C47.034 18.2121 39.1759 14.0758 36.777 14.9974C39.5287 14.6402 40.856 16.5364 41.0104 18.1106C41.2661 20.6639 39.3259 22.7541 26.0041 26.2289C12.5677 29.7258 10.5392 36.6006 10.5392 38.8848Z" fill="#fff"/>
        <path d="M11.2006 28.1649C11.2006 28.1649 9.4147 26.8905 9.4147 22.8556C9.4147 15.8133 19.5218 16.3733 19.5218 9.89102C19.5218 6.57492 15.6412 8.1536 16.3556 9.84252C15.7647 8.46228 17.63 7.11731 20.1347 7.13495C23.1951 7.16141 25.4925 9.7896 25.4925 12.9558C25.4925 21.6826 7.88452 19.5924 11.2006 28.1649Z" fill="#fff"/>
      </svg>
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
