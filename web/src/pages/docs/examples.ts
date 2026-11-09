import request from "./quickstart-request.json";

export function normalizeDocsBase(input: string): string | undefined {
  try {
    const url = new URL(input);
    if (
      url.username ||
      url.password ||
      url.search ||
      url.hash ||
      !/^\/[a-zA-Z0-9/_-]*v1\/?$/.test(url.pathname)
    )
      return;
    if (
      url.protocol !== "https:" &&
      !(
        url.protocol === "http:" &&
        ["localhost", "127.0.0.1", "[::1]"].includes(url.hostname)
      )
    )
      return;
    return `${url.origin}${url.pathname.replace(/\/$/, "")}`;
  } catch {
    return;
  }
}

export const bashQuote = (value: string) =>
  `'${value.replaceAll("'", `'"'"'`)}'`;
export const powershellQuote = (value: string) =>
  `'${value.replaceAll("'", "''")}'`;

export function quickstartExamples(base: string, model: string) {
  const body = JSON.stringify({ ...request, model }, null, 2);
  return {
    environment: `export MYAPIS_BASE_URL=${bashQuote(base)}
export MYAPIS_MODEL=${bashQuote(model)}
read -r -s -p "API key: " MYAPIS_API_KEY; printf '\\n'
export MYAPIS_API_KEY`,
    environmentWindows: `$env:MYAPIS_BASE_URL = ${powershellQuote(base)}
$env:MYAPIS_MODEL = ${powershellQuote(model)}
$secureKey = Read-Host 'API key' -AsSecureString
$env:MYAPIS_API_KEY = [System.Net.NetworkCredential]::new('', $secureKey).Password`,
    models: `curl --fail-with-body --silent --show-error "$MYAPIS_BASE_URL/models" \\
  -H "Authorization: Bearer $MYAPIS_API_KEY"`,
    modelsWindows: `Invoke-RestMethod -Uri "$env:MYAPIS_BASE_URL/models" -Headers @{
  Authorization = "Bearer $env:MYAPIS_API_KEY"
} | ConvertTo-Json -Depth 10`,
    curl: `curl --fail-with-body --silent --show-error \\
  "$MYAPIS_BASE_URL/chat/completions" \\
  -H "Authorization: Bearer $MYAPIS_API_KEY" \\
  -H "Content-Type: application/json" \\
  --data-raw ${bashQuote(body)}`,
    powershell: `$body = ${powershellQuote(body)}
$request = @{
  Method = 'Post'
  Uri = "$env:MYAPIS_BASE_URL/chat/completions"
  Headers = @{ Authorization = "Bearer $env:MYAPIS_API_KEY" }
  ContentType = 'application/json'
  Body = [System.Text.Encoding]::UTF8.GetBytes($body)
}
Invoke-RestMethod @request | ConvertTo-Json -Depth 10`,
    install: "python -m pip install openai",
    python: `import os
from openai import OpenAI

client = OpenAI(
    api_key=os.environ["MYAPIS_API_KEY"],
    base_url=os.environ["MYAPIS_BASE_URL"],
    max_retries=0,
    timeout=60.0,
)
response = client.chat.completions.create(
    model=os.environ["MYAPIS_MODEL"],
    messages=[{"role": "user", "content": "Say hello"}],
    max_tokens=64,
    stream=False,
)
print(response.choices[0].message.content)`,
    response: JSON.stringify(
      {
        id: "chatcmpl-example",
        object: "chat.completion",
        model,
        choices: [
          {
            index: 0,
            message: { role: "assistant", content: "Hello!" },
            finish_reason: "stop",
          },
        ],
      },
      null,
      2,
    ),
  };
}

export function jsonRequest(base: string, path: string, body: unknown) {
  return `curl --fail-with-body --silent --show-error ${bashQuote(base + path)} \\
  -H "Authorization: Bearer $MYAPIS_API_KEY" \\
  -H "Content-Type: application/json" \\
  --data-raw ${bashQuote(JSON.stringify(body, null, 2))}`;
}
