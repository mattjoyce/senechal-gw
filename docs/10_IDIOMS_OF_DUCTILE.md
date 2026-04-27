---
audience: [1, 2]
form: reference
density: expert
verified: 2026-04-27
---

# 10 Idioms of Ductile

1. **If it can be queued, it should be queued.**
   Queues make work deterministic, debuggable, and composable.

2. **Workflow logic belongs in the plugin.**
   Keep the core thin; plugins own orchestration and domain flow.

3. **Ductile is upstream and downstream.**
   It can be the trigger end or the action end in a pipeline.

4. **Events are the contract; payloads are the currency.**
   Stabilize event types and document payload schemas.

5. **Plugins own orchestration; core owns execution.**
   Core provides primitives; plugins assemble workflows.

6. **Composable over configurable.**
   Prefer small, chainable steps over giant option surfaces.

7. **Queues are the default boundary.**
   Cross-system or cross-service work should pass through a queue.

8. **Idempotent by design.**
   Every action should be safe to retry without side effects.

9. **Switch decides; plugins implement.**
   Central routing belongs in Switch; plugins handle domain logic.

10. **Observability is a feature.**
    Queued work must be traceable end-to-end.
