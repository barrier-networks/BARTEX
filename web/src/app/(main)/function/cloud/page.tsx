"use client";

import * as React from "react";

import { useRouter } from "next/navigation";

import { CloudIcon, KeyRoundIcon, ShieldCheckIcon } from "lucide-react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import { api } from "@/lib/api";
import type { TaskCredentialInput } from "@/lib/types";

// The Cloud Assessment launcher creates a read-only AWS assessment task, wired to
// the cloud-recon skill and Pacu (headless, read-only). Scoped keys are stored as
// a task credential (sealed at rest); account IDs and regions go into the goal so
// the agent enumerates only within the named scope.
export default function CloudAssessmentPage() {
  const router = useRouter();

  const [name, setName] = React.useState("");
  const [accounts, setAccounts] = React.useState("");
  const [regions, setRegions] = React.useState("us-east-1");
  const [credentialed, setCredentialed] = React.useState(true);
  const [accessKeyId, setAccessKeyId] = React.useState("");
  const [secretKey, setSecretKey] = React.useState("");
  const [sessionToken, setSessionToken] = React.useState("");
  const [submitting, setSubmitting] = React.useState(false);

  function parseList(s: string): string[] {
    return s
      .split(/[\n,]/)
      .map((x) => x.trim())
      .filter(Boolean);
  }

  function buildGoal(accountIds: string[], regionList: string[]): string {
    const scope = accountIds.length
      ? accountIds.join(", ")
      : "(no account IDs given — restrict to the operator's authorized account only)";
    const regionLine = regionList.length ? regionList.join(", ") : "us-east-1";
    const lines = [
      `Perform an AUTHORIZED, read-only AWS cloud security assessment of account(s): ${scope}. Regions: ${regionLine}.`,
      "",
      "1) External / anonymous: enumerate publicly exposed resources for the in-scope accounts (S3 buckets, public services, exposed endpoints) and confirm each exposure anonymously before reporting it.",
    ];
    if (credentialed) {
      lines.push(
        "2) Credentialed read: using the scoped read-only keys in this task's credentials (domain=aws), run a CSPM-style posture review across IAM, network, logging, encryption and public-access settings, and drive Pacu headlessly for read-only enumeration (whoami / enum modules) and IAM privilege-escalation PATH ANALYSIS only (iam__privesc_scan — no changes).",
      );
    }
    lines.push(
      "",
      "Read-only always: never Put/Create/Delete/Update/Attach, never AssumeRole into another account, never exfiltrate data, and enumerate only within the named account IDs. Report confirmed public exposure and high-risk misconfigurations with evidence.",
    );
    return lines.join("\n");
  }

  async function launch() {
    const accountIds = parseList(accounts);
    const regionList = parseList(regions);

    if (credentialed && (!accessKeyId.trim() || !secretKey.trim())) {
      toast.error("Credentialed assessment needs an access key ID and secret access key");
      return;
    }

    const credentials: TaskCredentialInput[] = [];
    if (credentialed) {
      const noteParts = ["AWS read-only, scoped"];
      if (accountIds.length) noteParts.push(`accounts=${accountIds.join(" ")}`);
      if (regionList.length) noteParts.push(`regions=${regionList.join(" ")}`);
      if (sessionToken.trim()) noteParts.push(`aws_session_token=${sessionToken.trim()}`);
      credentials.push({
        domain: "aws",
        username: accessKeyId.trim(),
        password: secretKey.trim(),
        note: noteParts.join("; "),
      });
    }

    const assessmentName = name.trim() || `AWS cloud assessment${accountIds.length ? ` — ${accountIds[0]}` : ""}`;
    setSubmitting(true);
    try {
      const task = await api.createTask({
        name: assessmentName,
        description: `Read-only AWS cloud assessment of account(s): ${accountIds.join(", ") || "operator-authorized account"}`,
        goal: buildGoal(accountIds, regionList),
        credentials,
      });
      toast.success("Cloud assessment task created");
      router.push(`/function/tasks/detail?id=${encodeURIComponent(task.id)}`);
    } catch (e) {
      toast.error(`Failed to create task: ${String(e)}`);
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="mx-auto flex w-full max-w-2xl flex-col gap-4 p-4">
      <div>
        <h1 className="flex items-center gap-2 text-xl font-semibold">
          <CloudIcon className="size-5" /> Cloud Assessment
        </h1>
        <p className="text-sm text-muted-foreground">
          Launch an authorized, read-only AWS assessment. External exposure is enumerated anonymously; with scoped
          read-only keys it adds a credentialed posture review and headless Pacu (read-only enumeration + IAM
          privilege-escalation path analysis). Keys are stored as a sealed task credential.
        </p>
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">Scope</CardTitle>
          <CardDescription>Only the named accounts are assessed. Keep this exact.</CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <div className="grid gap-2">
            <Label>Assessment name</Label>
            <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="Acme AWS — prod" />
          </div>
          <div className="grid gap-2">
            <Label>In-scope AWS account IDs (one per line or comma-separated)</Label>
            <Textarea
              className="font-mono text-xs"
              rows={3}
              value={accounts}
              onChange={(e) => setAccounts(e.target.value)}
              placeholder={"123456789012\n210987654321"}
            />
          </div>
          <div className="grid gap-2">
            <Label>Regions</Label>
            <Input value={regions} onChange={(e) => setRegions(e.target.value)} placeholder="us-east-1, eu-west-1" />
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <KeyRoundIcon className="size-4" /> Credentialed read
          </CardTitle>
          <CardDescription>
            Scoped, read-only keys for a CSPM posture review and headless Pacu. Leave off for external-only.
          </CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <div className="flex items-center justify-between rounded-md border p-3">
            <div className="space-y-0.5">
              <Label>Use scoped read-only credentials</Label>
              <p className="text-xs text-muted-foreground">Off = external / anonymous enumeration only.</p>
            </div>
            <Switch checked={credentialed} onCheckedChange={setCredentialed} />
          </div>
          {credentialed ? (
            <>
              <div className="grid gap-2">
                <Label>Access key ID</Label>
                <Input
                  value={accessKeyId}
                  onChange={(e) => setAccessKeyId(e.target.value)}
                  placeholder="AKIA…"
                  autoComplete="off"
                />
              </div>
              <div className="grid gap-2">
                <Label>Secret access key</Label>
                <Input
                  type="password"
                  value={secretKey}
                  onChange={(e) => setSecretKey(e.target.value)}
                  placeholder="••••••••"
                  autoComplete="off"
                />
              </div>
              <div className="grid gap-2">
                <Label>Session token (optional)</Label>
                <Textarea
                  className="font-mono text-xs"
                  rows={2}
                  value={sessionToken}
                  onChange={(e) => setSessionToken(e.target.value)}
                  placeholder="For temporary STS credentials"
                />
              </div>
              <p className="flex items-start gap-1.5 text-xs text-muted-foreground">
                <ShieldCheckIcon className="mt-0.5 size-3.5 shrink-0" />
                Use keys scoped to read-only (e.g. the AWS managed <code>SecurityAudit</code> /{" "}
                <code>ViewOnlyAccess</code>
                policies). The assessment never writes, deletes, or assumes roles into other accounts.
              </p>
            </>
          ) : null}
        </CardContent>
      </Card>

      <div className="flex justify-end gap-2">
        <Button onClick={() => void launch()} disabled={submitting}>
          {submitting ? "Creating…" : "Launch assessment"}
        </Button>
      </div>
    </div>
  );
}
