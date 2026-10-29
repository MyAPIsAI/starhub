// JSON submit-and-poll adapter for an OpenAI-compatible video upstream.
var meta = {
  apiVersion: 1,
  key: "openai-video",
  name: "OpenAI-compatible video",
  version: "1.0.0",
  models: ["sora-2", "sora-2-pro"],
  auth: "bearer"
};

function prepareSubmit(ctx) {
  var req = ctx.request;
  return {
    request: {method: "POST", path: "/v1/videos", body: {
      model: ctx.model, prompt: req.prompt, seconds: req.seconds, n: req.n, size: req.size
    }},
    usage: {seconds: req.seconds, n: req.n}
  };
}

function parseSubmit(ctx, response) {
  return {taskId: response.body.id};
}

function buildQuery(ctx) {
  return {method: "GET", path: "/v1/videos/" + encodeURIComponent(ctx.taskId)};
}

function parseQuery(ctx, response) {
  var body = response.body;
  var statuses = {queued: "submitted", pending: "submitted", in_progress: "in_progress",
    processing: "in_progress", completed: "succeeded", succeeded: "succeeded",
    failed: "failed", cancelled: "cancelled"};
  if (!statuses[body.status]) throw new Error("Unknown upstream status");
  var result = {status: statuses[body.status], progress: body.progress || 0,
    outputs: body.video && body.video.url ? [body.video.url] : [],
    fail_reason: body.error ? String(body.error.message || "Task failed") : ""};
  if (body.seconds) result.usage = {seconds: body.seconds, n: ctx.request.n};
  return result;
}
