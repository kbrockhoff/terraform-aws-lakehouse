---
name: iac-tasks-to-beads
description: Convert an iac-spec-kit (or spec-kit) tasks.md into beads (bd) issues, or re-sync beads after tasks.md changes. The whole tasks.md becomes one epic, each "## Phase N" becomes a feature under it, and each T### checklist item becomes a task under its phase, with each phase blocking the next. Use this whenever the user wants to load, import, push, convert, or turn tasks.md, specs/*/tasks.md, or the output of /iac.tasks into beads, bd issues, an epic, or the issue tracker, or wants checked-off tasks.md items reflected in beads ("sync tasks to beads", "create beads from the task list", "track the implementation in bd"). Also use it right after /iac.tasks finishes when the repo uses beads.
---

# iac-spec-kit tasks.md → beads

`/iac.tasks` writes an ordered checklist to `specs/<feature>/tasks.md`. Beads is where the team
claims and tracks that work. This skill bridges the two with one script, so the result is the same
on every run and a rerun syncs changes instead of creating duplicates.

## Mapping

| tasks.md | beads | Notes |
|---|---|---|
| The whole file (`# Tasks: <name>`) | **epic** `<name>` | Description: the preamble (Input, Organization, Path Conventions). Design: the trailing sections (Dependencies & Execution Order, and so on). Notes: the `## Notes` section. Labels: `iac-spec-kit` and `spec:<feature-dir>`, which every child inherits. |
| `## Phase N: <name>` | **feature** `Phase N: <name>` | Description: the phase prose (Purpose, CRITICAL notes) and its `###` task groups. Acceptance criteria: the `**Checkpoint**` line. Label: `phase:<n>`. Each feature blocks the next, so `bd ready` shows one phase at a time. A blocked feature also hides its child tasks. |
| `- [ ] T013 [P] …` | **task** `T013: <first clause>` | Description: the full task text, including indented sub-bullets, plus its source. Labels: `parallel` for `[P]`, `checkpoint` for checkpoint tasks, and story tags such as `[US1]` lowercased. A `[x]` item is created closed. |

Tasks get no task-to-task dependency edges, by design. Ordering inside a phase follows the
`[P]` markers and the epic's design notes, which carry the tasks.md dependency section. If the
user asks for finer-grained edges, add them afterward with `bd dep add`. Don't put them in the
plan.

Every issue has a stable `external_ref` (`iac-spec-kit:<feature-dir>`, `…#phase-<n>`, and
`…#T013`). That is how a rerun finds what already exists.

## Workflow

Run commands from the repository root, so the paths stored in issues are repo-relative.

### 1. Find tasks.md and check for beads

If the user didn't name a file, use the current feature's tasks.md:

```bash
.specify/scripts/bash/check-prerequisites.sh --json --paths-only   # prints TASKS
```

If that script is missing, use `specs/<current-branch>/tasks.md`. If there are several specs and
the choice is unclear, ask.

Confirm that a beads workspace exists (`bd list --limit 1` succeeds). If it doesn't, tell the user
and stop. Don't run `bd init` without asking, because it changes the repo setup.

### 2. Plan

```bash
python3 .agents/skills/iac-tasks-to-beads/scripts/tasks_to_beads.py plan specs/<feature>/tasks.md
```

This reads tasks.md and the beads already created from it, writes a plan directory (path
printed), and prints a summary. It changes nothing. The plan directory holds `summary.md`,
`plan.json`, and `graph.json`, which is the input for `bd create --graph`.

- **create mode**: no beads exist yet for this spec, so the plan is the whole tree.
- **sync mode**: beads exist, so the plan is only the delta:
  - **new** phases and tasks, attached to their existing parents. A new phase is linked into the
    phase chain.
  - **close**: tasks checked `[x]` in tasks.md whose beads are still open, and phases or the
    epic once all their tasks are done.
  - **reopen**: a phase or epic that was closed and now has open tasks again.
  - **drift**: reported only, nothing is changed. This covers beads whose title or description
    no longer matches tasks.md, tasks removed from tasks.md, and beads closed in bd while still
    unchecked in tasks.md.

Options:
- `--update-changed`: turn title and description drift into updates. Use it when the user wants
  beads to match edited task wording. Notes and comments on the beads are never touched.
- `--priority N`: priority for the epic and phases (default 2). A spec-kit phase title with
  `(Priority: P1)` sets that phase's priority automatically.
- `--offline`: skip the beads lookup. Useful for previewing a conversion.
- `--out DIR`: choose the plan directory.

If the script reports duplicate task IDs or phase numbers, tasks.md is malformed. Tell the user;
rerunning `/iac.tasks` or renumbering fixes it. Don't hand-edit the plan to work around it.

### 3. Review with the user

Show the summary: epic name, phase and task counts, and what will be created, closed, or
reopened. Include any drift lines. Skip the full task list unless it's short or the user asks for
it. A first-time conversion of a large file creates around 80 issues, and the user should see
that number before it happens.

If the user already said to go ahead ("convert it", "sync it"), continue without waiting. If
there is nothing to apply, say so and stop.

### 4. Apply

```bash
python3 .agents/skills/iac-tasks-to-beads/scripts/tasks_to_beads.py apply <plan-dir>
```

The apply step does the following, in order:
1. dry-runs the graph, so an invalid plan changes nothing;
2. reopens issues;
3. creates the issues in one `bd create --graph` call;
4. closes issues bottom-up, because bd won't close a parent that has open children;
5. applies updates.

It writes `result.json` with the new issue IDs, keyed by task ID.

### 5. Report

Tell the user:
- the epic ID and counts of created, closed, and reopened issues;
- what `bd ready` now shows, which is normally the first open phase and its tasks
  (`bd ready`, or `bd list --parent <epic-id>` for the tree);
- any drift, with a suggested action. For example: "T077 was removed from tasks.md; close
  `<id>` with `bd close <id> --reason 'dropped from tasks.md'` if that was intentional." Run such
  commands only if the user agrees, because drift can be intentional.

Don't commit, push, or run `bd dolt push` as part of this skill. Beads creation is local. Leave
syncing to the user or the project's session-close profile.

## Keeping both sides in step

`/iac.implement` checks tasks off in tasks.md. After an implementation session, rerun this skill
to close the matching beads. If someone closes a bead directly in bd, the next plan reports it as
drift ("closed in beads but unchecked in tasks.md"). The fix is to check the box in tasks.md
rather than reopen the bead, since tasks.md is the source document.
