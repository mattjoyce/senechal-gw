# Agent Development Workflow

## Simple Rule: One Card → One PR → Merge → Next Card

**Never work on multiple cards in the same branch.**

## Step-by-Step Process

### 1. Get Your Card Assignment
You will be assigned **one card** to work on.

Example: "Work on card #39"

### 2. Setup Your Branch
```bash
# Start from latest main
git checkout main
git pull origin main

# Create branch named: <agent>/<card-description>
git checkout -b claude/card39-multi-file-config
```

**Branch naming:** `<your-agent-name>/card<number>-<short-description>`
- claude/card39-multi-file-config
- codex/card36-manifest-metadata

### 3. Read the Card
```bash
# Find your card
ls kanban/*.md | xargs grep -l "^id: 39"

# Read it thoroughly
cat kanban/<card-file>.md
```

### 4. Do the Work
- Implement what the card describes
- Write tests
- Make sure tests pass: `go test ./...`
- Commit frequently with good messages

**Commit format:**
```
<component>: <verb> <what>

Example body text

Implements #39
```

### 5. Update the Card Status
When done, update the card file:

```yaml
---
id: 39
status: done  # Change from todo → done
---
```

Add a narrative entry at the bottom:
```markdown
## Narrative
- 2026-02-10: Implemented multi-file config with BLAKE3 verification. All tests passing. (by @claude)
```

### 6. Push and Create PR
```bash
# Add your changes including the card update
git add .
git commit -m "config: implement multi-file config system

Implements #39"

# Push your branch
git push -u origin claude/card39-multi-file-config

# Create PR
gh pr create \
  --title "config: implement multi-file config system (#39)" \
  --body "Implements #39

Summary of changes:
- Multi-file config directory
- BLAKE3 verification
- Cross-file validation

Tests passing: ✅"
```

### 7. Wait for PR to Merge
**STOP HERE.** Do not start the next card until:
- ✅ PR is reviewed
- ✅ PR is merged to main
- ✅ You receive your next card assignment

### 8. Get Next Card
After your PR merges:
```bash
# Clean up
git checkout main
git pull origin main
git branch -d claude/card39-multi-file-config

# Wait for next assignment
```

You will be told: "Work on card #42 next"

Then repeat from step 2.

## What NOT To Do

❌ **Don't work on multiple cards in one branch**
- Bad: claude/sprint3-all-my-work (contains #39, #42, #43)
- Good: claude/card39-multi-file-config (only #39)

❌ **Don't start next card before previous PR merges**
- Your next card might depend on the previous one merging
- Other agents might be blocked waiting for your work

❌ **Don't push without creating a PR**
- Pushing is not enough - create the PR immediately
- PRs are how work gets reviewed and merged

❌ **Don't forget to update the card status**
- Change `status: todo` → `status: done`
- Add narrative entry explaining what you did

## Questions?

**"What if my card is blocked by another card?"**
- Tell coordination immediately
- You'll get a different card to work on
- Never try to work around dependencies

**"What if I find a bug while working?"**
- If it's in your card's scope: fix it
- If it's outside your scope: report it, don't fix it

**"What if tests fail?"**
- Fix them before creating PR
- All tests must pass: `go test ./...`
- Never create a PR with failing tests

**"Can I work on the next card while waiting for PR review?"**
- No. Wait for merge.
- This prevents wasted work if PR needs changes

## Summary

1. Get card assignment
2. Create branch from main
3. Read card
4. Do work + tests
5. Update card status
6. Push + PR immediately
7. **WAIT for merge**
8. Get next card

**One card at a time. One PR at a time. Simple.**
