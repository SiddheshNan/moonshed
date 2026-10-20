const path = require("node:path");
const express = require("express");

const app = express();
const port = Number(process.env.PORT) || 3000;
const startedAt = Date.now();

// No database: the leaderboard lives in memory and resets when the app restarts or redeploys.
const MAX_SCORES = 10;
const scores = [];

app.use(express.json({ limit: "1kb" }));
app.use(express.static(path.join(__dirname, "public")));

app.get("/healthz", (_req, res) => res.send("ok"));

app.get("/api/info", (_req, res) => {
  res.json({
    app: process.env.APP_NAME || "node-snake",
    release: process.env.RELEASE_ID || "local",
    uptimeSeconds: Math.round((Date.now() - startedAt) / 1000),
  });
});

app.get("/api/scores", (_req, res) => res.json(scores));

app.post("/api/scores", (req, res) => {
  const name = String(req.body?.name ?? "").trim().slice(0, 16);
  const score = req.body?.score;
  if (!name) return res.status(400).json({ error: "name is required" });
  // One food is 10 points; a 20x20 board can't fit more than 400 of them.
  if (!Number.isInteger(score) || score <= 0 || score > 4000 || score % 10 !== 0) {
    return res.status(400).json({ error: "invalid score" });
  }
  scores.push({ name, score, at: new Date().toISOString() });
  scores.sort((a, b) => b.score - a.score || a.at.localeCompare(b.at));
  scores.length = Math.min(scores.length, MAX_SCORES);
  res.status(201).json(scores);
});

const server = app.listen(port, "0.0.0.0", () => {
  console.log(`snake listening on :${port}`);
});

// Moonshed stops the app on deploy; close cleanly so in-flight requests finish.
for (const signal of ["SIGTERM", "SIGINT"]) {
  process.on(signal, () => server.close(() => process.exit(0)));
}
