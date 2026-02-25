# TechTransfer Agency Bus: A Reliable Message Bus for Multi-Agent Workflows

## The open protocol that lets AI agents talk to each other — and lets humans watch

**For AI engineers and platform teams** building systems where multiple specialized agents need to coordinate work reliably. TechTransfer Agency Bus is a lightweight HTTP service that routes messages between agents, guarantees delivery, and gives operators full real-time visibility into every interaction.

---

## The Problem

Organizations are moving beyond single-agent AI. A patent screening workflow might need a document extractor, a domain expert, and a report generator — each a separate process with its own strengths. But wiring these agents together today means building bespoke point-to-point integrations, losing visibility into what's happening between them, and hoping nothing gets dropped.

There is no standard, simple way for agents to discover each other, exchange structured messages, and give operators confidence that work is progressing. Teams end up reinventing message routing, retry logic, and observability for every new multi-agent system they build.

## The Solution

TechTransfer Agency Bus is a single HTTP service that any agent can register with and communicate through. Agents send typed messages — requests, responses, and informational updates — organized into conversations. The bus handles delivery, acknowledgment tracking, and retry with backoff. Operators connect to a real-time event stream and see every message, every state change, and every error as it happens.

There are no SDKs to install. Any process that speaks HTTP can participate. Pull-mode agents long-poll an inbox. Push-mode agents receive callbacks. Both get at-least-once delivery with built-in idempotency. Authentication uses straightforward HMAC-SHA256 signatures.

## How It Works

A coordinator agent creates a conversation and sends a request to the first agent in a workflow. That agent does its work, posts progress events, and responds. The bus routes the response, the next agent picks it up, and the chain continues. Every step is visible through the observation stream — a dashboard, CLI tool, or logger can subscribe and watch the entire workflow unfold in real time.

The bus tracks message state through a clear lifecycle: pending, acknowledged, executing, completed or errored. If an agent doesn't acknowledge within the timeout, the bus marks it. Nothing is silently lost.

> "We built the TechTransfer Agency Bus because we needed our AI agents to collaborate the way good teams do — with clear handoffs, shared context, and full transparency into what's happening. The protocol is deliberately simple so that any HTTP-capable process can join, whether it's a Python script, a Go service, or a hosted LLM."
>
> — Joel Kehle, TechTransfer Agency

## A Real Workflow: Patent Screening

A university technology transfer office receives an invention disclosure as a PDF. Today, routing that through intake, extraction, expert evaluation, and report generation requires manual coordination or fragile scripts.

With the TechTransfer Agency Bus, five agents handle the entire pipeline autonomously. The coordinator creates a conversation, the intake agent validates the submission, the PDF extractor pulls the text, the patent agent evaluates eligibility, and the reporter assembles the final assessment. The technology transfer officer watches progress in real time through the event stream and sees the completed report — without writing any glue code.

> "I used to spend hours routing disclosures to the right reviewers and chasing status updates. Now I submit the PDF, watch the agents work through the event stream, and get a structured assessment back. The whole process is transparent — I can see exactly what each agent concluded and why."
>
> — Technology Transfer Officer

## Key Details

- **Protocol**: 13 HTTP endpoints. No proprietary SDKs or client libraries required.
- **Delivery**: At-least-once with request deduplication and configurable timeouts.
- **Observability**: Server-Sent Events stream carries every message, state change, and progress update in real time.
- **Security**: HMAC-SHA256 per-agent authentication. Agent and human allowlists.
- **Storage**: In-memory for development, snapshot-backed persistence for production.
- **Deployment**: Single Go binary. No external dependencies. Configure with environment variables.

## Get Started

```bash
# Start the bus
go run ./cmd/techtransfer-agency

# Run the patent screening demo end-to-end
go run ./cmd/patent-team --pdf disclosure.pdf --case-id CASE-2026-001
```

Register an agent, create a conversation, send a message. Three HTTP calls and your agent is participating in a multi-agent workflow with full observability and delivery guarantees.

---

## FAQ

### External FAQ

**Q: Do I need to use a specific language or framework?**
A: No. Any process that can make HTTP requests can be an agent. The protocol is language-agnostic.

**Q: How does this differ from a traditional message queue like RabbitMQ or Kafka?**
A: The TechTransfer Agency Bus is purpose-built for agent-to-agent workflows. It understands conversations, message types (request/response/inform), agent capabilities, and workflow state — not just raw message delivery. It's also much simpler to deploy and operate.

**Q: Can I use this in production?**
A: The persistent storage backend writes state snapshots to disk and is designed for production workloads. The protocol spec includes normative requirements for timeouts, idempotency windows, and error handling.

**Q: Can humans participate in workflows?**
A: Yes. Authorized humans can inject messages into any conversation through a dedicated endpoint. The bus authenticates human identities separately from agents.

### Internal FAQ

**Q: Why HTTP instead of gRPC or WebSockets?**
A: HTTP is the lowest common denominator. Every language, every platform, every hosted LLM provider can speak HTTP. The barrier to adding a new agent should be as close to zero as possible. SSE provides the real-time streaming layer without requiring full-duplex connections.

**Q: Why build this instead of using an existing orchestration framework?**
A: Existing frameworks (LangGraph, CrewAI, AutoGen) couple orchestration logic to a specific runtime and language. The TechTransfer Agency Bus separates the communication protocol from agent implementation, letting teams use whatever tools are best for each agent while maintaining a shared, observable communication layer.

**Q: What's the scaling story?**
A: The current implementation is a single-process, in-memory bus suitable for teams and workloads that don't require horizontal scaling. The protocol itself is stateless from the client's perspective and could be backed by a distributed store if needed.
