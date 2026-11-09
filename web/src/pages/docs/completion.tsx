import { useState, type ComponentType, type ReactNode } from "react";
import * as Tabs from "@radix-ui/react-tabs";
import { Link } from "react-router-dom";
import { documentationText } from "./content";
import { bashQuote, jsonRequest, quickstartExamples } from "./examples";
import "./docs.css";

type Copy = ReturnType<typeof documentationText>;
type Code = ComponentType<{ title: string; code: string }>;
export type ExtraArticle =
  "embeddings" | "images" | "audio" | "video" | "geminiCli";

export function Quickstart({
  copy,
  base,
  CodeBlock,
  connectionSettings,
}: {
  copy: Copy;
  base: string;
  CodeBlock: Code;
  connectionSettings: ReactNode;
}) {
  const [client, setClient] = useState("curl");
  const [pythonShell, setPythonShell] = useState(
    navigator.platform.toLowerCase().includes("win") ? "powershell" : "bash",
  );
  const [model, setModel] = useState("YOUR_AUTHORIZED_CHAT_MODEL");
  const samples = quickstartExamples(
    base,
    model.trim() || "YOUR_AUTHORIZED_CHAT_MODEL",
  );
  const windows =
    client === "powershell" ||
    (client === "python" && pythonShell === "powershell");
  return (
    <section className="docs-quickstart">
      <p className="docs-lead">{copy.startLead}</p>
      <div className="docs-callout docs-availability">
        <p>{copy.fundBoundary}</p>
      </div>

      <h2>{copy.prepareTitle}</h2>
      <ol className="docs-preparation">
        <li>
          {copy.prepareRegister} <Link to="/register">{copy.registerLink}</Link>
        </li>
        <li>
          {copy.prepareQuota} <Link to="/console">{copy.fundLink}</Link>
        </li>
        <li>
          {copy.prepareKey} <Link to="/console/keys">{copy.keysLink}</Link>
        </li>
      </ol>

      <h2>{copy.configureTitle}</h2>
      <p>{copy.configureHelp}</p>
      <div className="docs-connection-fields">
        {connectionSettings}
        <label className="docs-field">
          {copy.modelLabel}
          <input
            dir="ltr"
            value={model}
            onChange={(event) => setModel(event.target.value)}
            aria-describedby="docs-model-help"
            spellCheck={false}
          />
        </label>
      </div>
      <details className="docs-disclosure docs-model-discovery">
        <summary>{copy.chooseModel}</summary>
        <p id="docs-model-help">{copy.modelHelp}</p>
        <p>{copy.setupHelp}</p>
        <CodeBlock
          title={copy.listModels}
          code={windows ? samples.modelsWindows : samples.models}
        />
      </details>

      <h2>{copy.requestTitle}</h2>
      <p>{copy.requestHelp}</p>
      <Tabs.Root
        value={client}
        onValueChange={setClient}
        className="docs-client-examples"
      >
        <Tabs.List aria-label={copy.exampleTabs} className="docs-client-tabs">
          <Tabs.Trigger value="curl">cURL</Tabs.Trigger>
          <Tabs.Trigger value="python">Python</Tabs.Trigger>
          <Tabs.Trigger value="powershell">PowerShell</Tabs.Trigger>
        </Tabs.List>
        <div className="docs-environment">
          <details className="docs-disclosure">
            <summary>
              {copy.environment}{" "}
              <span>{windows ? copy.envWindows : copy.envBash}</span>
            </summary>
            <p>{copy.setupHelp}</p>
            {client === "python" && (
              <div
                className="docs-shell-options"
                role="group"
                aria-label={copy.environment}
              >
                {[
                  ["bash", "Bash · macOS / Linux"],
                  ["powershell", "PowerShell · Windows"],
                ].map(([id, title]) => (
                  <button
                    key={id}
                    type="button"
                    aria-pressed={pythonShell === id}
                    onClick={() => setPythonShell(id)}
                  >
                    {title}
                  </button>
                ))}
              </div>
            )}
            <CodeBlock
              title={copy.environment}
              code={windows ? samples.environmentWindows : samples.environment}
            />
          </details>
        </div>
        <Tabs.Content value="curl">
          <CodeBlock title={copy.send} code={samples.curl} />
        </Tabs.Content>
        <Tabs.Content value="powershell">
          <CodeBlock title={copy.send} code={samples.powershell} />
        </Tabs.Content>
        <Tabs.Content value="python">
          <p className="docs-sdk-help">{copy.sdkHelp}</p>
          <CodeBlock title="Terminal" code={samples.install} />
          <CodeBlock title="first_call.py" code={samples.python} />
          <a
            className="docs-action"
            href="https://github.com/openai/openai-python"
            target="_blank"
            rel="noreferrer"
          >
            {copy.official} · OpenAI Python
          </a>
        </Tabs.Content>
      </Tabs.Root>

      <h2>{copy.checkTitle}</h2>
      <p>{copy.successHelp}</p>
      <Link className="docs-action" to="/console/requests">
        {copy.requestsLink}
      </Link>
      <details className="docs-disclosure">
        <summary>{copy.successResponse}</summary>
        <CodeBlock title={copy.successResponse} code={samples.response} />
      </details>
      <p>{copy.troubleshoot}</p>
    </section>
  );
}

export function extraGuides(copy: Copy, base: string) {
  const root = base.replace(/\/v1$/, "");
  const upload = (endpoint: string, model: string, file: string, extra = "") =>
    `curl --fail-with-body --silent --show-error ${bashQuote(base + endpoint)} \\
  -H "Authorization: Bearer $MYAPIS_API_KEY" \\
  -F ${bashQuote(`model=${model}`)} \\
  -F ${bashQuote(file)}${extra}`;
  return {
    embeddings: {
      title: copy.embeddings,
      lead: copy.embeddingsHelp,
      examples: [
        jsonRequest(base, "/embeddings", {
          model: "YOUR_AUTHORIZED_EMBEDDING_MODEL",
          input: ["Hello", "Good morning"],
        }),
      ],
    },
    images: {
      title: copy.images,
      lead: copy.imagesHelp,
      examples: [
        jsonRequest(base, "/images/generations", {
          model: "YOUR_AUTHORIZED_IMAGE_MODEL",
          prompt: "A small blue boat on a calm lake",
          n: 1,
          size: "1024x1024",
        }),
        upload(
          "/images/edits",
          "YOUR_AUTHORIZED_IMAGE_MODEL",
          "image=@input.png",
          " \\\n  -F 'prompt=Make the boat green'",
        ),
      ],
    },
    audio: {
      title: copy.audio,
      lead: copy.audioHelp,
      examples: [
        jsonRequest(base, "/audio/speech", {
          model: "YOUR_AUTHORIZED_SPEECH_MODEL",
          input: "Hello!",
          voice: "alloy",
          response_format: "mp3",
        }) + " \\\n  --output speech.mp3",
        upload(
          "/audio/transcriptions",
          "YOUR_AUTHORIZED_TRANSCRIPTION_MODEL",
          "file=@sample.wav",
        ),
        upload(
          "/audio/translations",
          "YOUR_AUTHORIZED_TRANSLATION_MODEL",
          "file=@sample.wav",
        ),
      ],
    },
    video: {
      title: copy.video,
      lead: copy.videoHelp,
      examples: [
        jsonRequest(base, "/video/generations", {
          model: "YOUR_AUTHORIZED_VIDEO_MODEL",
          prompt: "A small blue boat on a calm lake",
          seconds: 4,
          n: 1,
        }),
        `TASK_ID='YOUR_PLATFORM_TASK_ID'
curl --fail-with-body --silent --show-error "${base}/video/generations/$TASK_ID" \\
  -H "Authorization: Bearer $MYAPIS_API_KEY"`,
      ],
    },
    geminiCli: {
      title: copy.geminiCli,
      lead: copy.geminiCliHelp,
      examples: [
        `npm install -g @google/gemini-cli
export GOOGLE_GEMINI_BASE_URL=${bashQuote(root)}
export GEMINI_API_KEY="$MYAPIS_API_KEY"
export GOOGLE_GENAI_API_VERSION='v1beta'
gemini --model 'YOUR_AUTHORIZED_GEMINI_MODEL'`,
      ],
    },
  };
}

export function Supplement({
  article,
  copy,
  base,
  CodeBlock,
}: {
  article: string;
  copy: Copy;
  base: string;
  CodeBlock: Code;
}) {
  const guides = extraGuides(copy, base);
  if (article in guides) {
    const guide = guides[article as ExtraArticle];
    const exampleLabels: Partial<Record<ExtraArticle, string[]>> = {
      audio: [copy.speech, copy.transcription, copy.translation],
      images: [copy.imageGenerate, copy.imageEdit],
      video: [copy.videoSubmit, copy.videoPoll],
    };
    const labels = exampleLabels[article as ExtraArticle];
    return (
      <section>
        <p className="docs-lead">{guide.lead}</p>
        <h2>{copy.exampleTabs}</h2>
        {labels ? (
          <Tabs.Root
            key={article}
            defaultValue="0"
            className="docs-client-examples docs-operation-examples"
          >
            <Tabs.List
              className="docs-client-tabs"
              aria-label={copy.exampleTabs}
            >
              {labels.map((label, index) => (
                <Tabs.Trigger key={label} value={String(index)}>
                  {label}
                </Tabs.Trigger>
              ))}
            </Tabs.List>
            {guide.examples.map((code, index) => (
              <Tabs.Content key={index} value={String(index)}>
                <CodeBlock title={`${labels[index]} · Bash`} code={code} />
              </Tabs.Content>
            ))}
          </Tabs.Root>
        ) : (
          guide.examples.map((code, index) => (
            <CodeBlock
              key={index}
              title={`${guide.title} · Bash`}
              code={code}
            />
          ))
        )}
        {article === "geminiCli" && (
          <a
            className="docs-action"
            href="https://geminicli.com/docs/reference/configuration/"
            target="_blank"
            rel="noreferrer"
          >
            {copy.official} · Gemini CLI
          </a>
        )}
      </section>
    );
  }
  const diagnostics = `curl --fail-with-body --silent --show-error --include ${bashQuote(base + "/models")} \\
  -H "Authorization: Bearer $MYAPIS_API_KEY"`;
  const supplements: Record<string, { lead: string; code: string }> = {
    auth: { lead: copy.authHelp, code: diagnostics },
    protocols: { lead: copy.diagnosticsHelp, code: diagnostics },
    tiers: { lead: copy.routingHelp, code: diagnostics },
    routing: {
      lead: copy.routingHelp,
      code: jsonRequest(base, "/chat/completions", {
        model: "myapis-auto",
        messages: [{ role: "user", content: "Say hello" }],
        max_tokens: 64,
        stream: false,
      }),
    },
    errors: { lead: copy.diagnosticsHelp, code: diagnostics },
    limits: { lead: copy.limitsHelp, code: diagnostics },
    faq: { lead: copy.diagnosticsHelp, code: diagnostics },
    billing: {
      lead: copy.billingHelp,
      code: `from decimal import Decimal

# Illustrative rates only; use the model and group rates at call time.
input_rate = Decimal("1.00")
output_rate = Decimal("4.00")
input_tokens = Decimal("1000")
output_tokens = Decimal("250")
usd = (input_tokens * input_rate + output_tokens * output_rate) / Decimal("1000000")
print(usd)  # 0.00200 USD`,
    },
  };
  const supplement = supplements[article];
  return supplement ? (
    <section>
      <p>{supplement.lead}</p>
      <CodeBlock
        title={`${copy.example} · ${article === "billing" ? "Python" : "Bash"}`}
        code={supplement.code}
      />
    </section>
  ) : null;
}
