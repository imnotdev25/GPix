import { useState } from "react";
import type { Gateways } from "../types";
import Icon from "./Icon";
import { useConfirm, useToast } from "./feedback";

type Client = "aws" | "rclone" | "boto3";
const clients: { id: Client; label: string }[] = [
  { id: "aws", label: "AWS CLI" },
  { id: "rclone", label: "rclone" },
  { id: "boto3", label: "Python" },
];

function CopyBtn({ label, onClick }: { label: string; onClick: () => void }) {
  return (
    <button className="btn btn-ghost btn-icon btn-sm" onClick={onClick} aria-label={`Copy ${label}`} title="Copy">
      <Icon name="copy" size={15} />
    </button>
  );
}

/** The S3 gateway: status, connection details, keys, and ready-to-run client setup. */
export default function S3Card({
  g,
  act,
}: {
  g: Gateways;
  act: (path: string, body: unknown, ok: string) => Promise<void>;
}) {
  const [reveal, setReveal] = useState(false);
  const [client, setClient] = useState<Client>("aws");
  const ask = useConfirm();
  const toast = useToast();

  const copy = async (text: string, what: string) => {
    try {
      await navigator.clipboard.writeText(text);
      toast(`${what} copied`, "ok");
    } catch {
      toast(`Couldn't copy the ${what.toLowerCase()}`, "err");
    }
  };

  const regenerate = async () => {
    if (g.HasS3Keys) {
      const ok = await ask({
        title: "Make new S3 keys?",
        message: "The current keys stop working right away. Clients using them need the new pair.",
        confirmLabel: "Make new keys",
        danger: true,
      });
      if (!ok) return;
    }
    setReveal(true);
    await act("/api/v1/gateways/regenerate", { kind: "s3" }, "New S3 keys made. Copy the secret key into your client now.");
  };

  const clear = async () => {
    const ok = await ask({
      title: "Remove the S3 keys?",
      message: "S3 clients can't connect until you make new keys.",
      confirmLabel: "Remove keys",
      danger: true,
    });
    if (ok) await act("/api/v1/gateways/clear", { kind: "s3" }, "S3 keys removed.");
  };

  const ep = g.S3Endpoint;
  const b = g.S3Bucket;
  const rg = g.S3Region;
  const ak = g.S3AccessKey;
  const sk = reveal ? g.S3SecretKey : "<secret key>";
  const masked = reveal ? g.S3SecretKey : "•".repeat(Math.min(Math.max(g.S3SecretKey.length, 8), 32));
  const snippets: Record<Client, string> = {
    aws: `aws configure set aws_access_key_id ${ak} --profile gpix
aws configure set aws_secret_access_key ${sk} --profile gpix
aws configure set region ${rg} --profile gpix
aws configure set s3.addressing_style path --profile gpix

aws --profile gpix --endpoint-url ${ep} s3 ls s3://${b}/
aws --profile gpix --endpoint-url ${ep} s3 cp ./photo.jpg s3://${b}/`,
    rclone: `rclone config create gpix s3 provider Other \\
  access_key_id ${ak} secret_access_key ${sk} \\
  endpoint ${ep} region ${rg} force_path_style true

rclone ls gpix:${b}
rclone copy ./photos gpix:${b}`,
    boto3: `import boto3
from botocore.config import Config

s3 = boto3.client(
    "s3",
    endpoint_url="${ep}",
    aws_access_key_id="${ak}",
    aws_secret_access_key="${sk}",
    region_name="${rg}",
    config=Config(s3={"addressing_style": "path"}),
)
for obj in s3.list_objects_v2(Bucket="${b}").get("Contents", []):
    print(obj["Key"], obj["Size"])`,
  };

  return (
    <div className="card">
      <div className="row">
        <h2 style={{ margin: 0 }}>S3</h2>
        <span className={"pill" + (g.S3Enabled ? " on" : "")}>{g.S3Enabled ? "Running" : "Off"}</span>
      </div>
      <p className="muted">
        An S3-compatible endpoint for your library, for the AWS CLI, rclone, Cyberduck or any S3 SDK.
        Listing comes from gpix's cached library, so it's quick; a file is only downloaded and
        decrypted when a client reads it.
      </p>
      {!g.S3Enabled && (
        <p className="muted">
          To turn it on, set <code>GPIX_S3_LISTEN=0.0.0.0:9000</code> in <code>.env</code> and restart
          gpix. You can make keys now.
        </p>
      )}

      {(g.S3Enabled || g.HasS3Keys) && (
        <div className="kv">
          {g.S3Enabled && (
            <>
              <span className="kv-k">Endpoint</span>
              <code className="kv-v">{ep}</code>
              <CopyBtn label="endpoint" onClick={() => copy(ep, "Endpoint")} />
              <span className="kv-k">Bucket</span>
              <code className="kv-v">{b}</code>
              <CopyBtn label="bucket" onClick={() => copy(b, "Bucket name")} />
              <span className="kv-k">Region</span>
              <code className="kv-v">{rg}</code>
              <span />
            </>
          )}
          {g.HasS3Keys && (
            <>
              <span className="kv-k">Access key</span>
              <code className="kv-v">{ak}</code>
              <CopyBtn label="access key" onClick={() => copy(ak, "Access key")} />
              <span className="kv-k">Secret key</span>
              <code className="kv-v">{masked}</code>
              <span className="row nowrap" style={{ gap: 2 }}>
                <button
                  className="btn btn-ghost btn-icon btn-sm"
                  onClick={() => setReveal((v) => !v)}
                  aria-label={reveal ? "Hide secret key" : "Show secret key"}
                  title={reveal ? "Hide" : "Show"}
                >
                  <Icon name={reveal ? "eyeOff" : "eye"} size={15} />
                </button>
                <CopyBtn label="secret key" onClick={() => copy(g.S3SecretKey, "Secret key")} />
              </span>
            </>
          )}
        </div>
      )}
      {!g.HasS3Keys && <p className="muted">No keys yet. Make a key pair to connect a client.</p>}

      <div className="row">
        <button className="btn btn-primary" onClick={regenerate}>
          <Icon name="key" size={16} />
          {g.HasS3Keys ? "Make new keys" : "Make keys"}
        </button>
        {g.HasS3Keys && (
          <button className="btn" onClick={clear}>
            Remove keys
          </button>
        )}
      </div>

      {g.S3Enabled && g.HasS3Keys && (
        <>
          <h3 className="subhead">Connect a client</h3>
          <div className="segmented" role="tablist" aria-label="Client">
            {clients.map((c) => (
              <button
                key={c.id}
                type="button"
                role="tab"
                aria-selected={client === c.id}
                className={client === c.id ? "on" : ""}
                onClick={() => setClient(c.id)}
              >
                {c.label}
              </button>
            ))}
          </div>
          <div className="snippet">
            <pre>
              <code>{snippets[client]}</code>
            </pre>
            <button className="btn btn-sm" onClick={() => copy(snippets[client], "Commands")}>
              <Icon name="copy" size={14} />
              Copy
            </button>
          </div>
          {!reveal && (
            <p className="muted">The secret key is left out of these commands; show it above to fill it in.</p>
          )}
        </>
      )}
    </div>
  );
}
