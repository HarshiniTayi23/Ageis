# Learning path

What to understand, milestone by milestone, well enough to explain in an interview without notes. Tick a box only when you could teach it to someone.

## Foundations (before M1 ends)
- [ ] **Go basics**: structs, interfaces, errors, goroutines, channels, `context`. Start with [A Tour of Go](https://go.dev/tour/), then read `internal/repo/repo.go` line by line.
- [ ] **Content-defined chunking**: why fixed blocks break dedup after an insertion. Read `internal/chunker` and the FastCDC paper (Xia et al., USENIX ATC 2016).
- [ ] **Authenticated encryption**: what AES-GCM's tag protects, and why the object key is passed as associated data (`internal/seal`).
- [ ] **Hash chains**: how one hash per snapshot makes a deleted snapshot detectable.
- [ ] **Object storage and Object Lock**: governance vs compliance mode, and why compliance mode beats a stolen root key.
- [ ] **Docker**: images, layers, multi-stage builds, why distroless.

## M2 · Agent undo
- [ ] **How AI agents call tools**: the Model Context Protocol (MCP). Read `internal/mcp/mcp.go` (about 150 lines), then run `make demo-agent` and look at the JSON lines it sends.
- [ ] **Event sourcing**: why the journal never updates anything, and how "pending" is derived by replay (`internal/journal`, ADR-0007).
- [ ] **Optimistic concurrency**: undo only reverts if the file's hash still matches what the agent left (`agentfs.Undo`).
- [ ] **AWS Signature V4**: read `internal/store/sigv4.go` against AWS's examples in `sigv4_test.go`.
- [ ] **Change data capture**: Postgres write-ahead log and logical replication.
- [ ] **Identity**: how an agent's credentials differ from a human's, and least privilege.

## M3 · Distributed and AI
- [ ] **Raft consensus**: leader election and log replication. Read the paper "In Search of an Understandable Consensus Algorithm"; try the [Raft visualisation](https://thesecretlivesofdata.com/raft/).
- [ ] **Kubernetes**: Deployments, Services, probes, Secrets, CronJobs (`deploy/k8s`).
- [ ] **Model evaluation**: precision, recall, confusion matrices, choosing a threshold.
- [ ] **LLM grounding**: retrieval, verifying claims, measuring hallucination.

## Product management
- [ ] Jobs to be done and personas
- [ ] Writing a PRD with testable requirements
- [ ] Metrics, counter-metrics and baselines
- [ ] Prioritisation: RICE and MoSCoW
- [ ] AI evaluation plans and ship bars
