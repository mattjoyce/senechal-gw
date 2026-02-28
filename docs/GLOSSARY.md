# Ductile Glossary: The Integration Codex

Ductile is a **lightweight, open-source integration engine for the agentic era.** To operate it effectively, whether you are a human architect or an LLM Operator, you must understand the **"Integration Sphere"**—the functional foundation upon which all agentic affordances are built.

This glossary provides a **robust compound semantic grounding**, bridging traditional systems engineering with the unique requirements of LLM-native execution.

---

## Core Integration Primitives (The Human View)

These terms represent the "Functional Reality" of the engine. They are grounded in the mechanics of how data and execution move through the system.

### **Gateway**
The central integration host and runtime (the `ductile` binary). It manages the lifecycle of connectors, the orchestration of events, and the integrity of the execution ledger.

### **Plugin (Connector)**
A polyglot adapter that bridges Ductile to an external system (e.g., an API, a database, or a shell environment). Connectors are the source of all functional capabilities.

### **Command (Operation)**
A discrete, low-level action provided by a Connector. 
- **`poll` (Discovery):** A proactive integration pattern where Ductile pulls data from a source on a schedule.
- **`handle` (Reactive):** A reactive integration pattern where Ductile processes an incoming event.
- **`health` (Diagnostic):** A verification pattern to ensure a Connector's prerequisites are met.

### **Pipeline (Orchestration)**
A defined sequence of operations triggered by an event. Orchestrations define how data flows between multiple connectors to achieve a complex functional goal.

### **Event**
A typed packet of data (e.g., `file.changed` or `webhook.received`) that signals a state change and triggers a routing decision within the Gateway.

### **Job**
The unit of work. Every time a Command is executed, an immutable **Job** record is created in the ledger, capturing the input, output, logs, and status.

---

## Agentic Era Extensions (The LX View)

These terms represent the **"Abstraction Layer"** designed for the LLM Operator. They map the core integration primitives to the reasoning requirements of an agent.

### **Skill (Affordance)**
An LLM-facing projection of a core **Command**. While a human sees a "poll operation," an LLM sees a "Discovery Skill." This is the **LX (LLM Experience)** layer of the engine.

### **Baggage (Working Memory)**
Stateful metadata (JSON) that persists across a multi-hop Pipeline. It allows the LLM to maintain "context" (like a user ID or a goal) as it moves through disparate integration steps.

### **Execution Ledger (The Paper Trail)**
The persistent history of all Jobs and transitions. For an LLM, the ledger is a searchable record of past actions, providing the "Lineage" needed to reason about the current state of the world.

### **Workspace (Audit Trail)**
An isolated file system directory created for a specific Job. It ensures that an LLM's side effects are bounded and verifiable by a human operator.

### **Inference Frugality**
A design principle focused on reducing "inference friction." By providing a self-describing environment (`--skills`), Ductile ensures the LLM can execute perfectly on the first try, saving tokens and increasing reliability.

---

## Summary Mapping

| Integration Sphere (Functional) | Agentic Era (LX Abstraction) | Why it matters |
| :--- | :--- | :--- |
| **Connector** | **Capability Source** | Where the power comes from. |
| **Operation** | **Skill** | What the agent can actually *do*. |
| **Orchestration** | **Reasoning Chain** | How the agent achieves complex goals. |
| **Baggage** | **Working Memory** | How the agent stays on track. |
| **Ledger** | **Experience/Memory** | How the agent learns from the past. |
| **Workspace** | **Action Sandbox** | Where the agent safely "touches" the world. |
