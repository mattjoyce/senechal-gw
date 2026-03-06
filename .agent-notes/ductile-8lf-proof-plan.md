# ductile-8lf proof plan

## Question to prove
When the synchronous API returns for a pipeline whose entry step is skipped, does the downstream child job already exist in `job_queue`?

## Proof strategy
- Add focused API/integration-level test covering synchronous root-triggered skipped entry step
- Capture sync API response tree length/status
- Immediately inspect actual DB state / queue tree after response
- Optionally poll for a short interval to detect delayed child enqueue visibility

## Outcome interpretation
- Response tree missing child, DB already has child => waiter/response assembly bug
- Response tree missing child, DB gets child only later => timing/order/race bug in skip continuation
- Response tree missing child, DB never gets child => continuation bug
