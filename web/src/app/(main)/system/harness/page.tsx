"use client";

import * as React from "react";

import {
  CheckCircle2Icon,
  KeyRoundIcon,
  LogInIcon,
  PlusIcon,
  RefreshCwIcon,
  ShieldIcon,
  TerminalSquareIcon,
  Trash2Icon,
} from "lucide-react";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { NativeSelect, NativeSelectOption } from "@/components/ui/native-select";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import { api } from "@/lib/api";
import type { HarnessProfile, HarnessProfileInput } from "@/lib/types";

type Kind = "codex" | "claude-code" | "custom";
type Transport = "stdio" | "http";

type FormState = {
  name: string;
  kind: Kind;
  command: string;
  args: string; // one arg per line
  env: string; // KEY=VALUE per line
  mcp_transport: Transport;
  disable_native_sandbox: boolean;
  daybreak: boolean;
  max_duration_seconds: string;
};

// Sensible starting points per the research doc (docs/harness-cli-profiles.md).
const PRESETS: Record<Kind, Partial<FormState>> = {
  codex: {
    command: "codex",
    // `codex exec`, non-interactive, externally sandboxed (BARTEX's container is
    // the boundary). The bartex MCP server is injected inline via -c overrides so
    // the per-run endpoint + token travel with the invocation (codex reads
    // mcp_servers from config, not a --mcp-config file).
    args: [
      "exec",
      "--dangerously-bypass-approvals-and-sandbox",
      "-c",
      'mcp_servers.bartex.command="{{SELF_EXE}}"',
      "-c",
      'mcp_servers.bartex.args=["mcp-stdio"]',
      "-c",
      'mcp_servers.bartex.env.BARTEX_MCP_ENDPOINT="{{MCP_ENDPOINT}}"',
      "-c",
      'mcp_servers.bartex.env.BARTEX_MCP_TOKEN="{{MCP_TOKEN}}"',
      "-c",
      "daybreak=true",
      "{{PROMPT}}",
    ].join("\n"),
    mcp_transport: "stdio",
    daybreak: true,
  },
  "claude-code": {
    command: "claude",
    args: [
      "-p",
      "{{PROMPT}}",
      "--mcp-config",
      "{{MCP_CONFIG}}",
      "--strict-mcp-config",
      "--allowedTools",
      "mcp__bartex__*",
    ].join("\n"),
    mcp_transport: "stdio",
    daybreak: false,
  },
  custom: { command: "", args: "{{PROMPT}}", mcp_transport: "stdio", daybreak: false },
};

const emptyForm: FormState = {
  name: "",
  kind: "codex",
  command: "codex",
  args: PRESETS.codex.args ?? "",
  env: "",
  mcp_transport: "stdio",
  disable_native_sandbox: true,
  daybreak: true,
  max_duration_seconds: "",
};

function parseEnv(text: string): Record<string, string> {
  const out: Record<string, string> = {};
  for (const line of text.split("\n")) {
    const t = line.trim();
    if (!t || t.startsWith("#")) continue;
    const i = t.indexOf("=");
    if (i <= 0) continue;
    out[t.slice(0, i).trim()] = t.slice(i + 1);
  }
  return out;
}

function toInput(f: FormState, includeEnv: boolean): HarnessProfileInput {
  const input: HarnessProfileInput = {
    name: f.name.trim(),
    kind: f.kind,
    command: f.command.trim(),
    args: f.args
      .split("\n")
      .map((s) => s.replace(/\r$/, "").trim())
      .filter((s) => s !== ""),
    mcp_transport: f.mcp_transport,
    disable_native_sandbox: f.disable_native_sandbox,
    daybreak: f.daybreak,
    max_turns: 0,
    max_duration_seconds: Number(f.max_duration_seconds) || 0,
  };
  if (includeEnv) input.env = parseEnv(f.env);
  return input;
}

export default function HarnessPage() {
  const [profiles, setProfiles] = React.useState<HarnessProfile[]>([]);
  const [loading, setLoading] = React.useState(true);
  const [open, setOpen] = React.useState(false);
  const [editing, setEditing] = React.useState<HarnessProfile | null>(null);
  const [form, setForm] = React.useState<FormState>(emptyForm);
  const [saving, setSaving] = React.useState(false);
  const [defaultId, setDefaultId] = React.useState<number | null>(null);

  const load = React.useCallback(async () => {
    setLoading(true);
    try {
      const [ps, def] = await Promise.all([
        api.harnessProfiles(),
        api.harnessDefault().catch(() => ({ harness_profile_id: null })),
      ]);
      setProfiles(ps);
      setDefaultId(def.harness_profile_id ?? null);
    } catch (e) {
      toast.error(`Failed to load harness profiles: ${String(e)}`);
    } finally {
      setLoading(false);
    }
  }, []);

  async function changeDefault(value: string) {
    const id = value === "" ? null : Number(value);
    try {
      await api.setHarnessDefault(id);
      setDefaultId(id);
      toast.success(id == null ? "Default backend set to built-in loop" : "Default execution backend updated");
    } catch (e) {
      toast.error(`Failed to set default: ${String(e)}`);
    }
  }

  React.useEffect(() => {
    void load();
  }, [load]);

  function openCreate() {
    setEditing(null);
    setForm(emptyForm);
    setOpen(true);
  }

  function openEdit(p: HarnessProfile) {
    setEditing(p);
    setForm({
      name: p.name,
      kind: p.kind,
      command: p.command,
      args: p.args.join("\n"),
      env: (p.env_keys ?? []).map((k) => `${k}=`).join("\n"), // values are sealed; re-enter to change
      mcp_transport: p.mcp_transport,
      disable_native_sandbox: p.disable_native_sandbox,
      daybreak: p.daybreak,
      max_duration_seconds: p.max_duration_seconds ? String(p.max_duration_seconds) : "",
    });
    setOpen(true);
  }

  function applyKind(kind: Kind) {
    setForm((f) => ({ ...f, kind, ...PRESETS[kind] }) as FormState);
  }

  async function save() {
    if (!form.name.trim() || !form.command.trim()) {
      toast.error("Name and command are required");
      return;
    }
    setSaving(true);
    try {
      // On edit, only send env when the operator typed at least one value (a
      // bare KEY= with no value is treated as "leave it"); otherwise omit env so
      // the sealed values are kept.
      const envHasValues = form.env
        .split("\n")
        .some((l) => l.includes("=") && l.split("=").slice(1).join("=").trim() !== "");
      if (editing) {
        await api.updateHarnessProfile(editing.id, toInput(form, envHasValues));
      } else {
        await api.createHarnessProfile(toInput(form, true));
      }
      toast.success(editing ? "Profile updated" : "Profile created");
      setOpen(false);
      await load();
    } catch (e) {
      toast.error(`Save failed: ${String(e)}`);
    } finally {
      setSaving(false);
    }
  }

  async function remove(p: HarnessProfile) {
    if (!confirm(`Delete harness profile "${p.name}"? Tasks using it revert to the built-in backend.`)) return;
    try {
      await api.deleteHarnessProfile(p.id);
      toast.success("Profile deleted");
      await load();
    } catch (e) {
      toast.error(`Delete failed: ${String(e)}`);
    }
  }

  return (
    <div className="flex flex-col gap-4 p-4">
      <div className="flex items-center justify-between gap-2">
        <div>
          <h1 className="flex items-center gap-2 text-xl font-semibold">
            <TerminalSquareIcon className="size-5" /> Harness Backends
          </h1>
          <p className="max-w-3xl text-sm text-muted-foreground">
            Run a task on an external agentic CLI (Codex/Daybreak, Claude Code, …) instead of the built-in LLM loop. In
            whole-task mode the CLI plans and executes the whole engagement through the governed BARTEX Tools MCP
            server, confined by the OS sandbox with egress forced through the recording proxy. Attach a profile per task
            from its detail page, or set a default below.
          </p>
        </div>
        <div className="flex gap-2">
          <Button variant="outline" size="sm" onClick={() => void load()}>
            <RefreshCwIcon className="size-4" /> Refresh
          </Button>
          <Button size="sm" onClick={openCreate}>
            <PlusIcon className="size-4" /> New profile
          </Button>
        </div>
      </div>

      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="text-base">Default execution backend</CardTitle>
          <CardDescription>
            Applies to tasks with no explicit backend. Set a harness here to run every new task on the CLI with no LLM
            provider configured; leave it on the built-in loop to require an LLM.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <NativeSelect
            className="max-w-sm"
            value={defaultId == null ? "" : String(defaultId)}
            onChange={(e) => void changeDefault(e.target.value)}
          >
            <NativeSelectOption value="">Built-in loop (requires an LLM)</NativeSelectOption>
            {profiles.map((p) => (
              <NativeSelectOption key={p.id} value={String(p.id)}>
                {p.name} ({p.kind})
              </NativeSelectOption>
            ))}
          </NativeSelect>
        </CardContent>
      </Card>

      <CodexAuthCard />

      {loading ? (
        <p className="text-sm text-muted-foreground">Loading…</p>
      ) : profiles.length === 0 ? (
        <Card>
          <CardContent className="py-8 text-center text-sm text-muted-foreground">
            No harness profiles yet. Create one to run a task on an external CLI.
          </CardContent>
        </Card>
      ) : (
        <div className="grid gap-3 md:grid-cols-2">
          {profiles.map((p) => (
            <Card key={p.id} className="cursor-pointer hover:border-primary/40" onClick={() => openEdit(p)}>
              <CardHeader className="flex flex-row items-center justify-between gap-2 pb-2">
                <CardTitle className="flex items-center gap-2 text-base">
                  {p.name}
                  <Badge variant="secondary">{p.kind}</Badge>
                  {p.daybreak ? <Badge variant="outline">daybreak</Badge> : null}
                </CardTitle>
                <Button
                  variant="ghost"
                  size="icon"
                  onClick={(e) => {
                    e.stopPropagation();
                    void remove(p);
                  }}
                >
                  <Trash2Icon className="size-4 text-destructive" />
                </Button>
              </CardHeader>
              <CardContent className="space-y-1 text-sm text-muted-foreground">
                <div className="font-mono text-xs break-all">
                  {p.command} {p.args.join(" ")}
                </div>
                <div className="flex flex-wrap gap-2 pt-1">
                  <Badge variant="outline">MCP: {p.mcp_transport}</Badge>
                  <Badge variant={p.disable_native_sandbox ? "outline" : "destructive"} className="gap-1">
                    <ShieldIcon className="size-3" />
                    {p.disable_native_sandbox ? "BARTEX sandbox" : "native sandbox"}
                  </Badge>
                  {p.max_duration_seconds > 0 ? <Badge variant="outline">{p.max_duration_seconds}s cap</Badge> : null}
                  {(p.env_keys?.length ?? 0) > 0 ? (
                    <Badge variant="outline">env: {p.env_keys!.join(", ")}</Badge>
                  ) : null}
                </div>
              </CardContent>
            </Card>
          ))}
        </div>
      )}

      <Sheet open={open} onOpenChange={setOpen}>
        <SheetContent className="flex w-full flex-col gap-4 overflow-y-auto sm:max-w-xl">
          <SheetHeader>
            <SheetTitle>{editing ? "Edit harness profile" : "New harness profile"}</SheetTitle>
            <SheetDescription>
              Placeholders in args are substituted at launch: <code>{"{{PROMPT}}"}</code>,{" "}
              <code>{"{{MCP_CONFIG}}"}</code> (stdio), <code>{"{{MCP_URL}}"}</code> (http), <code>{"{{RUN_DIR}}"}</code>
              .
            </SheetDescription>
          </SheetHeader>

          <div className="flex flex-col gap-4 px-4 pb-6">
            <div className="grid gap-2">
              <Label>Name</Label>
              <Input
                value={form.name}
                onChange={(e) => setForm({ ...form, name: e.target.value })}
                placeholder="codex-daybreak"
              />
            </div>

            <div className="grid grid-cols-2 gap-3">
              <div className="grid gap-2">
                <Label>Kind</Label>
                <NativeSelect value={form.kind} onChange={(e) => applyKind(e.target.value as Kind)}>
                  <NativeSelectOption value="codex">codex</NativeSelectOption>
                  <NativeSelectOption value="claude-code">claude-code</NativeSelectOption>
                  <NativeSelectOption value="custom">custom</NativeSelectOption>
                </NativeSelect>
              </div>
              <div className="grid gap-2">
                <Label>MCP transport</Label>
                <NativeSelect
                  value={form.mcp_transport}
                  onChange={(e) => setForm({ ...form, mcp_transport: e.target.value as Transport })}
                >
                  <NativeSelectOption value="stdio">stdio (bartex mcp-stdio)</NativeSelectOption>
                  <NativeSelectOption value="http">http (/api/mcp)</NativeSelectOption>
                </NativeSelect>
              </div>
            </div>

            <div className="grid gap-2">
              <Label>Command</Label>
              <Input
                value={form.command}
                onChange={(e) => setForm({ ...form, command: e.target.value })}
                placeholder="codex"
              />
            </div>

            <div className="grid gap-2">
              <Label>Args (one per line)</Label>
              <Textarea
                className="font-mono text-xs"
                rows={7}
                value={form.args}
                onChange={(e) => setForm({ ...form, args: e.target.value })}
              />
            </div>

            <div className="grid gap-2">
              <Label>Env (KEY=VALUE per line)</Label>
              <Textarea
                className="font-mono text-xs"
                rows={3}
                value={form.env}
                onChange={(e) => setForm({ ...form, env: e.target.value })}
                placeholder="OPENAI_API_KEY=sk-..."
              />
              {editing ? (
                <p className="text-xs text-muted-foreground">
                  Values are sealed and never shown. Leave a key blank (<code>KEY=</code>) to keep its stored value;
                  type a new value to replace it.
                </p>
              ) : null}
            </div>

            <div className="flex items-center justify-between rounded-md border p-3">
              <div className="space-y-0.5">
                <Label className="flex items-center gap-1">
                  <ShieldIcon className="size-4" /> Confine with BARTEX sandbox
                </Label>
                <p className="text-xs text-muted-foreground">
                  Run the CLI with its own sandbox off, inside BARTEX&apos;s. Required — launching is refused if no OS
                  sandbox is available.
                </p>
              </div>
              <Switch
                checked={form.disable_native_sandbox}
                onCheckedChange={(v) => setForm({ ...form, disable_native_sandbox: v })}
              />
            </div>

            <div className="grid grid-cols-2 gap-3">
              <div className="flex items-center justify-between rounded-md border p-3">
                <Label>Daybreak (Codex)</Label>
                <Switch checked={form.daybreak} onCheckedChange={(v) => setForm({ ...form, daybreak: v })} />
              </div>
              <div className="grid gap-2">
                <Label>Max duration (s)</Label>
                <Input
                  type="number"
                  value={form.max_duration_seconds}
                  onChange={(e) => setForm({ ...form, max_duration_seconds: e.target.value })}
                  placeholder="0 = unlimited"
                />
              </div>
            </div>

            <div className="flex justify-end gap-2 pt-2">
              <Button variant="outline" onClick={() => setOpen(false)}>
                Cancel
              </Button>
              <Button onClick={() => void save()} disabled={saving}>
                {saving ? "Saving…" : editing ? "Save changes" : "Create profile"}
              </Button>
            </div>
          </div>
        </SheetContent>
      </Sheet>
    </div>
  );
}

// CodexAuthCard manages Codex CLI authentication inside the container (System →
// Harness). Device-code sign-in shows a URL + one-time code to complete in a
// browser; an API-key path is offered as an alternative. Auth persists in the
// container's CODEX_HOME volume.
function CodexAuthCard() {
  const [loggedIn, setLoggedIn] = React.useState(false);
  const [detail, setDetail] = React.useState("");
  const [pending, setPending] = React.useState<{ url: string; code: string } | null>(null);
  const [busy, setBusy] = React.useState(false);
  const [showKey, setShowKey] = React.useState(false);
  const [apiKey, setApiKey] = React.useState("");

  const refresh = React.useCallback(async () => {
    try {
      const st = await api.codexStatus();
      setLoggedIn(st.logged_in);
      setDetail(st.detail ?? "");
      if (st.pending) {
        setPending({ url: st.pending.url, code: st.pending.code });
      } else {
        setPending(null);
      }
      if (st.logged_in) setPending(null);
    } catch {
      /* harness auth is optional; ignore load errors */
    }
  }, []);

  React.useEffect(() => {
    void refresh();
  }, [refresh]);

  // While a device login is pending, poll status so the card flips to "signed in"
  // once the operator authorizes in their browser.
  React.useEffect(() => {
    if (!pending) return;
    const t = setInterval(() => void refresh(), 4000);
    return () => clearInterval(t);
  }, [pending, refresh]);

  async function startDevice() {
    setBusy(true);
    try {
      const r = await api.codexDeviceLogin();
      setPending({ url: r.url, code: r.code });
      toast.success("Open the link and enter the code to finish sign-in");
    } catch (e) {
      toast.error(`Device sign-in failed: ${String(e)}`);
    } finally {
      setBusy(false);
    }
  }

  async function saveKey() {
    if (!apiKey.trim()) return;
    setBusy(true);
    try {
      await api.codexApiKeyLogin(apiKey.trim());
      setApiKey("");
      setShowKey(false);
      toast.success("Codex authenticated with API key");
      await refresh();
    } catch (e) {
      toast.error(`API-key sign-in failed: ${String(e)}`);
    } finally {
      setBusy(false);
    }
  }

  async function logout() {
    setBusy(true);
    try {
      await api.codexLogout();
      setPending(null);
      await refresh();
      toast.success("Codex signed out");
    } catch (e) {
      toast.error(`Sign-out failed: ${String(e)}`);
    } finally {
      setBusy(false);
    }
  }

  return (
    <Card>
      <CardHeader className="pb-3">
        <CardTitle className="flex items-center gap-2 text-base">
          <KeyRoundIcon className="size-4" /> Codex authentication
          {loggedIn ? (
            <Badge variant="secondary" className="gap-1">
              <CheckCircle2Icon className="size-3" /> Signed in
            </Badge>
          ) : (
            <Badge variant="outline">Not signed in</Badge>
          )}
        </CardTitle>
        <CardDescription>
          Sign Codex in inside the container so the harness backend can run. Credentials persist across rebuilds.
          {detail ? <span className="ml-1 font-mono text-xs">· {detail}</span> : null}
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-3">
        {pending ? (
          <div className="rounded-md border bg-muted/40 p-3 text-sm">
            <p className="mb-2 text-muted-foreground">Finish sign-in in your browser:</p>
            <ol className="ml-4 list-decimal space-y-1">
              <li>
                Open{" "}
                <a href={pending.url} target="_blank" rel="noreferrer" className="font-medium text-primary underline">
                  {pending.url}
                </a>
              </li>
              <li>
                Enter this one-time code (expires in 15 min):{" "}
                <code className="rounded bg-background px-1.5 py-0.5 font-mono text-base tracking-widest">
                  {pending.code}
                </code>
              </li>
            </ol>
            <p className="mt-2 text-xs text-muted-foreground">
              Waiting for authorization… this card updates automatically.
            </p>
          </div>
        ) : null}

        <div className="flex flex-wrap gap-2">
          {!loggedIn ? (
            <Button size="sm" onClick={() => void startDevice()} disabled={busy}>
              <LogInIcon className="size-4" /> Sign in with ChatGPT
            </Button>
          ) : null}
          <Button size="sm" variant="outline" onClick={() => setShowKey((v) => !v)} disabled={busy}>
            <KeyRoundIcon className="size-4" /> Use API key
          </Button>
          <Button size="sm" variant="outline" onClick={() => void refresh()} disabled={busy}>
            <RefreshCwIcon className="size-4" /> Refresh
          </Button>
          {loggedIn ? (
            <Button size="sm" variant="ghost" onClick={() => void logout()} disabled={busy}>
              Sign out
            </Button>
          ) : null}
        </div>

        {showKey ? (
          <div className="flex items-end gap-2">
            <div className="grid flex-1 gap-1">
              <Label>OpenAI API key</Label>
              <Input
                type="password"
                value={apiKey}
                onChange={(e) => setApiKey(e.target.value)}
                placeholder="sk-…"
                autoComplete="off"
              />
            </div>
            <Button size="sm" onClick={() => void saveKey()} disabled={busy || !apiKey.trim()}>
              Save
            </Button>
          </div>
        ) : null}
        <p className="text-xs text-muted-foreground">
          Daybreak Red requires a ChatGPT sign-in; device-code sign-in must be enabled in your ChatGPT security
          settings. API-key sign-in uses API billing and some Daybreak features may require ChatGPT auth.
        </p>
      </CardContent>
    </Card>
  );
}
