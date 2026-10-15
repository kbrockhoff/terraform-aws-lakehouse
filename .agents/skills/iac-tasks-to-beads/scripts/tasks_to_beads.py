#!/usr/bin/env python3
"""Convert an iac-spec-kit tasks.md into a beads (bd) issue tree, or sync an existing one.

Shape of the tree:
    epic     <- the whole tasks.md            external_ref iac-spec-kit:<spec>
    feature  <- each "## Phase X: ..."        external_ref iac-spec-kit:<spec>#phase-<X>
    task     <- each "- [ ] T### ..."         external_ref iac-spec-kit:<spec>#T###

Each phase feature blocks the next one, so `bd ready` surfaces one phase at a time
(a blocked feature also hides its child tasks). There are no task-to-task edges.

Subcommands:
    plan  <tasks.md> [--out DIR] [--offline] [--update-changed] [--priority N]
          Parse tasks.md, look up beads already created from it, write
          DIR/graph.json (input for `bd create --graph`), DIR/plan.json, and
          DIR/summary.md. Makes no changes to beads.
    apply <DIR>
          Execute a plan: create new issues, close finished ones, apply updates.

Only the Python standard library is used. `bd` must be on PATH (unless --offline).
"""

from __future__ import annotations

import argparse
import json
import os
import re
import subprocess
import sys
import tempfile
from dataclasses import dataclass, field
from pathlib import Path

SOURCE = "iac-spec-kit"
TITLE_MAX = 100

TASK_RE = re.compile(r"^\s*-\s+\[(?P<done>[ xX])\]\s+(?P<id>T\d+)\b\s*(?P<rest>.*)$")
TAG_RE = re.compile(r"^\[(?P<tag>[^\]]+)\]\s*")
PHASE_RE = re.compile(r"^##\s+Phase\s+(?P<num>[^:\s]+)\s*:\s*(?P<name>.+?)\s*$")
H1_RE = re.compile(r"^#\s+(?:Tasks:\s*)?(?P<name>.+?)\s*$")
H2_RE = re.compile(r"^##\s+(?P<name>.+?)\s*$")
H3_RE = re.compile(r"^###\s+(?P<name>.+?)\s*$")
PRIORITY_RE = re.compile(r"\(Priority:\s*P(?P<p>[0-4])\)", re.IGNORECASE)
BOILERPLATE_SECTIONS = {"format: `[id] [p?] description`", "format: [id] [p?] description"}


@dataclass
class Task:
    tid: str
    done: bool
    tags: list[str]
    first_line: str
    body: list[str]
    group: str | None

    @property
    def parallel(self) -> bool:
        return "P" in self.tags

    @property
    def checkpoint(self) -> bool:
        return self.first_line.lstrip("*").lower().startswith("checkpoint")


@dataclass
class Phase:
    num: str
    name: str
    prose: list[str] = field(default_factory=list)
    tasks: list[Task] = field(default_factory=list)

    @property
    def title(self) -> str:
        return f"Phase {self.num}: {self.name}"

    @property
    def checkpoint_text(self) -> str | None:
        for line in self.prose:
            m = re.match(r"^\*\*Checkpoint\*\*:?\s*(.+)$", line.strip())
            if m:
                return m.group(1).strip()
        return None

    @property
    def priority(self) -> int | None:
        m = PRIORITY_RE.search(self.name)
        return int(m.group("p")) if m else None


@dataclass
class TasksDoc:
    path: Path
    spec: str
    name: str
    preamble: list[str]
    phases: list[Phase]
    trailing: list[tuple[str, list[str]]]  # (section heading, lines) after the last phase


# ---------------------------------------------------------------- parsing


def strip_comments_and_frontmatter(text: str) -> str:
    text = re.sub(r"<!--.*?-->", "", text, flags=re.DOTALL)
    # YAML front matter: the template opens with "---\n\ndescription: ...\n---".
    m = re.match(r"^---\s*\n(.*?)\n---\s*\n", text, flags=re.DOTALL)
    if m and "description:" in m.group(1):
        text = text[m.end():]
    return text


def clean_block(lines: list[str]) -> str:
    """Trim blank edges, drop horizontal rules, collapse runs of blank lines."""
    out: list[str] = []
    for line in lines:
        if line.strip() == "---":
            continue
        if not line.strip() and (not out or not out[-1].strip()):
            continue
        out.append(line.rstrip())
    while out and not out[-1].strip():
        out.pop()
    return "\n".join(out)


def dedent_body(lines: list[str]) -> list[str]:
    indents = [len(l) - len(l.lstrip()) for l in lines if l.strip()]
    cut = min(indents) if indents else 0
    return [l[cut:] if l.strip() else "" for l in lines]


def parse(path: Path) -> TasksDoc:
    text = strip_comments_and_frontmatter(path.read_text(encoding="utf-8"))
    lines = text.splitlines()

    name = path.parent.name
    preamble: list[str] = []
    phases: list[Phase] = []
    trailing: list[tuple[str, list[str]]] = []

    current_phase: Phase | None = None
    current_trailing: list[str] | None = None
    group: str | None = None
    task: Task | None = None
    in_fence = False
    seen_h1 = False

    def close_task() -> None:
        nonlocal task
        if task is not None:
            task.body = dedent_body(task.body)
            while task.body and not task.body[-1].strip():
                task.body.pop()
            task = None

    for idx, line in enumerate(lines, start=1):
        if line.strip().startswith("```"):
            in_fence = not in_fence
        if in_fence or line.strip().startswith("```"):
            # Fenced code belongs to whatever block we are in.
            if task is not None:
                task.body.append(line)
            elif current_phase is not None:
                current_phase.prose.append(line)
            elif current_trailing is not None:
                current_trailing.append(line)
            else:
                preamble.append(line)
            continue

        if not seen_h1:
            m = H1_RE.match(line)
            if m and not line.startswith("##"):
                name = m.group("name")
                seen_h1 = True
                continue

        m = PHASE_RE.match(line)
        if m:
            close_task()
            current_phase = Phase(num=m.group("num"), name=m.group("name"))
            phases.append(current_phase)
            current_trailing = None
            group = None
            continue

        m = H2_RE.match(line)
        if m and not line.startswith("###"):
            close_task()
            section = m.group("name")
            if phases:  # a non-phase H2 after phases started ends phase parsing
                current_phase = None
                current_trailing = []
                trailing.append((section, current_trailing))
            else:
                preamble.append(line)
            group = None
            continue

        m = TASK_RE.match(line)
        if m and current_trailing is None:
            close_task()
            rest = m.group("rest")
            tags: list[str] = []
            while True:
                tm = TAG_RE.match(rest)
                if not tm:
                    break
                tags.append(tm.group("tag").strip())
                rest = rest[tm.end():]
            if current_phase is None:
                current_phase = Phase(num="0", name="Unphased")
                phases.append(current_phase)
            task = Task(
                tid=m.group("id"),
                done=m.group("done").lower() == "x",
                tags=tags,
                first_line=rest.strip(),
                body=[],
                group=group,
            )
            current_phase.tasks.append(task)
            continue

        if current_phase is not None:
            m = H3_RE.match(line)
            if m:
                close_task()
                group = m.group("name")
                continue

        # Continuation of a task: indented lines, or blank lines that sit between
        # indented lines.
        if task is not None:
            if line.strip() == "":
                task.body.append("")
                continue
            if line[:1] in (" ", "\t"):
                task.body.append(line)
                continue
            close_task()

        if current_phase is not None:
            current_phase.prose.append(line)
        elif current_trailing is not None:
            current_trailing.append(line)
        else:
            preamble.append(line)

    close_task()
    return TasksDoc(path=path, spec=path.parent.name, name=name, preamble=preamble,
                    phases=phases, trailing=trailing)


# ---------------------------------------------------------------- rendering


def clause_cut(text: str, limit: int) -> int | None:
    """Index of the first clause boundary (". ", ":", ";") outside code spans and brackets."""
    depth, in_code = 0, False
    for i, ch in enumerate(text[:limit]):
        if ch == "`":
            in_code = not in_code
        elif in_code:
            continue
        elif ch in "([{":
            depth += 1
        elif ch in ")]}":
            depth = max(0, depth - 1)
        elif depth == 0 and i >= 15 and (ch in ":;" or (ch == "." and text[i + 1:i + 2] == " ")):
            return i
    return None


def short_title(text: str, limit: int = TITLE_MAX) -> str:
    """First clause of a task line, so titles stay scannable in `bd list`."""
    text = re.sub(r"\s+", " ", text.replace("**", "").strip()).rstrip(":").strip()
    cut = clause_cut(text, limit)
    if cut is not None:
        return text[:cut].rstrip(" .")
    if len(text) <= limit:
        return text
    cut = text.rfind(" ", 0, limit - 1)
    return text[: cut if cut > 20 else limit - 1].rstrip(" ,(") + "…"


def task_title(t: Task) -> str:
    first = t.first_line.replace("**", "").strip()
    prefix = f"{t.tid}: "
    if t.checkpoint:
        # "**Checkpoint**: run `make check`" -> "Checkpoint: run `make check`"
        rest = re.sub(r"^Checkpoint\s*:\s*", "", first, flags=re.IGNORECASE)
        if rest != first:
            return prefix + "Checkpoint: " + short_title(rest, TITLE_MAX - 12)
    return prefix + short_title(first)


def preamble_text(doc: TasksDoc) -> str:
    """Preamble minus the boilerplate Format section."""
    out: list[str] = []
    skipping = False
    for line in doc.preamble:
        m = H2_RE.match(line)
        if m and not line.startswith("###"):
            skipping = m.group("name").strip().lower() in BOILERPLATE_SECTIONS
            if skipping:
                continue
        if not skipping:
            out.append(line)
    return clean_block(out)


def rel(path: Path) -> str:
    try:
        return str(path.resolve().relative_to(Path.cwd().resolve()))
    except ValueError:
        return str(path)


def build_nodes(doc: TasksDoc, base_priority: int) -> list[dict]:
    """Every node the tasks.md implies, in creation order, keyed by external_ref."""
    src = rel(doc.path)
    spec_dir = rel(doc.path.parent)
    epic_ref = f"{SOURCE}:{doc.spec}"
    nodes: list[dict] = []

    design_parts, notes_parts = [], []
    for heading, lines in doc.trailing:
        block = clean_block(lines)
        if not block:
            continue
        if heading.strip().lower() == "notes":
            notes_parts.append(block)
        else:
            design_parts.append(f"## {heading}\n\n{block}")

    pre = preamble_text(doc)
    epic_desc = f"Implementation tasks for `{spec_dir}`, generated from `{src}`.\n"
    epic_desc += "Each phase is a child feature, and each feature blocks the next phase.\n"
    if pre:
        epic_desc += "\n" + pre
    nodes.append({
        "key": "epic",
        "title": doc.name,
        "type": "epic",
        "priority": base_priority,
        "description": epic_desc,
        "design": "\n\n".join(design_parts),
        "notes": "\n\n".join(notes_parts),
        "labels": [SOURCE, f"spec:{doc.spec}"],
        "external_ref": epic_ref,
        "spec_id": spec_dir,
        "metadata": {"source": SOURCE, "tasks_file": src},
        "_done": bool(doc.phases) and all(t.done for p in doc.phases for t in p.tasks),
    })

    for p in doc.phases:
        fkey = f"phase-{p.num}"
        prio = p.priority if p.priority is not None else base_priority
        desc = clean_block(p.prose) or "(no phase description in tasks.md)"
        groups = [g for g in dict.fromkeys(t.group for t in p.tasks) if g]
        if groups:
            desc += "\n\nTask groups:\n" + "\n".join(f"- {g}" for g in groups)
        desc += f"\n\nSource: `{src}`, {p.title}"
        nodes.append({
            "key": fkey,
            "parent_key": "epic",
            "title": p.title,
            "type": "feature",
            "priority": prio,
            "description": desc,
            "acceptance_criteria": p.checkpoint_text or "",
            "labels": [f"phase:{p.num.lower()}"],
            "external_ref": f"{epic_ref}#{fkey}",
            "spec_id": spec_dir,
            "metadata": {"source": SOURCE, "tasks_file": src, "phase": p.num},
            "_done": bool(p.tasks) and all(t.done for t in p.tasks),
        })
        for t in p.tasks:
            body = t.first_line
            if t.body:
                body += "\n" + "\n".join(t.body)
            where = p.title + (f" > {t.group}" if t.group else "")
            desc = f"{body}\n\nSource: `{src}` ({where})"
            labels = []
            if t.parallel:
                labels.append("parallel")
            if t.checkpoint:
                labels.append("checkpoint")
            labels += [tag.lower() for tag in t.tags if tag != "P"]
            meta = {"source": SOURCE, "tasks_file": src, "task_id": t.tid, "phase": p.num}
            if t.group:
                meta["group"] = t.group
            nodes.append({
                "key": t.tid,
                "parent_key": fkey,
                "title": task_title(t),
                "type": "task",
                "priority": prio,
                "description": desc,
                "labels": labels,
                "external_ref": f"{epic_ref}#{t.tid}",
                "spec_id": spec_dir,
                "metadata": meta,
                "_done": t.done,
            })
    return nodes


# ---------------------------------------------------------------- bd access


def bd(*args: str) -> str:
    proc = subprocess.run(["bd", *args], capture_output=True, text=True)
    if proc.returncode != 0:
        detail = "\n".join(x for x in (proc.stderr.strip(), proc.stdout.strip()) if x)
        raise SystemExit(f"bd {' '.join(args[:2])} failed ({proc.returncode}):\n{detail}")
    return proc.stdout


def existing_issues(spec: str) -> dict[str, dict]:
    out = bd("list", "--all", "--json", "--limit", "0", "--external-contains", f"{SOURCE}:{spec}")
    data = json.loads(out or "[]")
    if isinstance(data, dict):
        data = data.get("issues", [])
    prefix = f"{SOURCE}:{spec}"
    found: dict[str, dict] = {}
    for issue in data:
        ref = issue.get("external_ref") or ""
        # Exact spec match: "<prefix>" or "<prefix>#..." (not "<prefix>-other").
        if ref == prefix or ref.startswith(prefix + "#"):
            found[ref] = issue
    return found


# ---------------------------------------------------------------- planning


def make_plan(doc: TasksDoc, existing: dict[str, dict], update_changed: bool,
              base_priority: int) -> dict:
    nodes = build_nodes(doc, base_priority)
    by_key = {n["key"]: n for n in nodes}
    ref_of = {n["key"]: n["external_ref"] for n in nodes}
    feature_keys = [n["key"] for n in nodes if n["type"] == "feature"]

    def existing_id(key: str) -> str | None:
        issue = existing.get(ref_of[key])
        return issue["id"] if issue else None

    new_nodes: list[dict] = []
    closes: list[dict] = []
    reopens: list[dict] = []
    updates: list[dict] = []
    drift: list[str] = []

    for n in nodes:
        issue = existing.get(n["external_ref"])
        if issue is None:
            node = {k: v for k, v in n.items() if not k.startswith("_") and v not in ("", None, [])}
            if "parent_key" in node:
                pid = existing_id(node["parent_key"])
                if pid:
                    node.pop("parent_key")
                    node["parent_id"] = pid
            if n["_done"]:
                node["status"] = "closed"
            new_nodes.append(node)
            continue

        status = issue.get("status")
        if n["_done"] and status != "closed":
            closes.append({"id": issue["id"], "key": n["key"], "type": n["type"],
                           "title": issue.get("title")})
        elif not n["_done"] and status == "closed":
            if n["type"] == "task":
                drift.append(f"{issue['id']} ({n['key']}) is closed in beads but unchecked in tasks.md")
            else:
                # A phase or epic closed because all its tasks were done, and it now has
                # open work again (for example, a task was added to it).
                reopens.append({"id": issue["id"], "key": n["key"], "type": n["type"],
                                 "title": issue.get("title")})

        changes = {}
        for fld in ("title", "description"):
            if (issue.get(fld) or "").strip() != (n.get(fld) or "").strip():
                changes[fld] = n.get(fld) or ""
        if changes:
            if update_changed:
                updates.append({"id": issue["id"], "key": n["key"], **changes})
            else:
                drift.append(f"{issue['id']} ({n['key']}) differs from tasks.md in: "
                             + ", ".join(sorted(changes)))

    wanted = {n["external_ref"] for n in nodes}
    for ref, issue in sorted(existing.items()):
        if ref not in wanted and issue.get("status") != "closed":
            drift.append(f"{issue['id']} ({ref.split('#')[-1]}) no longer appears in tasks.md "
                         "(not deleted; close or supersede it if it was dropped)")

    # Phase chain: each feature depends on the previous one. Only edges that touch a
    # new feature are emitted; edges between existing features already exist.
    new_keys = {n["key"] for n in new_nodes}
    edges: list[dict] = []
    for prev, cur in zip(feature_keys, feature_keys[1:]):
        if cur not in new_keys and prev not in new_keys:
            continue
        edge: dict = {"type": "blocks"}
        edge.update({"from_key": cur} if cur in new_keys else {"from_id": existing_id(cur)})
        edge.update({"to_key": prev} if prev in new_keys else {"to_id": existing_id(prev)})
        edges.append(edge)

    return {
        "tasks_file": rel(doc.path),
        "spec": doc.spec,
        "mode": "sync" if existing else "create",
        "graph": {"nodes": new_nodes, "edges": edges},
        "closes": closes,
        "reopens": reopens,
        "updates": updates,
        "drift": drift,
        # Readable names for keys and existing ids, used by the summary.
        "names": {**{i["id"]: i.get("title", "") for i in existing.values()},
                  **{n["key"]: n["title"] for n in nodes}},
        "counts": {
            "phases": len(doc.phases),
            "tasks": sum(len(p.tasks) for p in doc.phases),
            "tasks_done": sum(t.done for p in doc.phases for t in p.tasks),
            "existing": len(existing),
        },
        "_by_key": by_key,
    }


def summary_md(plan: dict, doc: TasksDoc) -> str:
    c = plan["counts"]
    g = plan["graph"]
    names = plan.get("names", {})
    lines = [f"# Beads plan for `{plan['tasks_file']}` ({plan['mode']})", ""]
    lines.append(f"- Epic: **{doc.name}**")
    lines.append(f"- {c['phases']} phases, {c['tasks']} tasks ({c['tasks_done']} checked off)")
    if plan["mode"] == "sync":
        lines.append(f"- {c['existing']} beads already exist for this spec")
    lines.append(f"- Will create {len(g['nodes'])} issues and {len(g['edges'])} phase edges; "
                 f"reopen {len(plan['reopens'])}; close {len(plan['closes'])}; update {len(plan['updates'])}")
    lines.append("")

    if g["nodes"]:
        lines.append("## New issues")
        lines.append("")
        for n in g["nodes"]:
            indent = {"epic": "", "feature": "  ", "task": "    "}[n["type"]]
            mark = " (closed)" if n.get("status") == "closed" else ""
            labels = [l for l in n.get("labels", []) if l in ("parallel", "checkpoint")]
            extra = f" [{', '.join(labels)}]" if labels else ""
            under = f" (under {names.get(n['parent_id'], n['parent_id'])})" if "parent_id" in n else ""
            lines.append(f"{indent}- [{n['type']}] {n['title']}{extra}{mark}{under}")
        lines.append("")
    if g["edges"]:
        lines.append("## Phase ordering")
        lines.append("")
        for e in g["edges"]:
            src_, dst = e.get("from_key") or e.get("from_id"), e.get("to_key") or e.get("to_id")
            lines.append(f"- {names.get(src_, src_)} waits for {names.get(dst, dst)}")
        lines.append("")
    if plan["reopens"]:
        lines.append("## Reopen (new open tasks under a closed parent)")
        lines.append("")
        lines += [f"- {r['id']} {r['title']}" for r in plan["reopens"]]
        lines.append("")
    if plan["closes"]:
        lines.append("## Close (checked off in tasks.md)")
        lines.append("")
        lines += [f"- {c['id']} {c['title']}" for c in plan["closes"]]
        lines.append("")
    if plan["updates"]:
        lines.append("## Update from tasks.md")
        lines.append("")
        lines += [f"- {u['id']} ({u['key']}): {', '.join(k for k in u if k in ('title', 'description'))}"
                  for u in plan["updates"]]
        lines.append("")
    if plan["drift"]:
        lines.append("## Drift (reported only, no action taken)")
        lines.append("")
        lines += [f"- {d}" for d in plan["drift"]]
        lines.append("")
    if not (g["nodes"] or plan["reopens"] or plan["closes"] or plan["updates"]):
        lines.append("Beads already match tasks.md; there is nothing to apply.")
        lines.append("")
    return "\n".join(lines)


# ---------------------------------------------------------------- commands


def cmd_plan(args: argparse.Namespace) -> int:
    path = Path(args.tasks_md)
    if not path.is_file():
        raise SystemExit(f"not a file: {path}")
    doc = parse(path)
    if not doc.phases:
        raise SystemExit(f"no '## Phase N: ...' sections or T### tasks found in {path}")
    ids = [t.tid for p in doc.phases for t in p.tasks]
    dupes = sorted({i for i in ids if ids.count(i) > 1})
    if dupes:
        raise SystemExit(f"duplicate task IDs in {path}: {', '.join(dupes)}. "
                         "Renumber them (rerun /iac.tasks) before converting.")
    nums = [p.num for p in doc.phases]
    dupes = sorted({n for n in nums if nums.count(n) > 1})
    if dupes:
        raise SystemExit(f"duplicate phase numbers in {path}: {', '.join(dupes)}")

    existing = {} if args.offline else existing_issues(doc.spec)
    plan = make_plan(doc, existing, args.update_changed, args.priority)

    out = Path(args.out) if args.out else Path(tempfile.mkdtemp(prefix=f"beads-plan-{doc.spec}-"))
    out.mkdir(parents=True, exist_ok=True)
    plan.pop("_by_key")
    (out / "graph.json").write_text(json.dumps(plan["graph"], indent=2) + "\n")
    (out / "plan.json").write_text(json.dumps(plan, indent=2) + "\n")
    summary = summary_md(plan, doc)
    (out / "summary.md").write_text(summary)

    print(summary)
    print(f"Plan written to: {out}")
    print(f"  graph.json  -> bd create --graph {out / 'graph.json'} --dry-run")
    print(f"  apply with  -> python3 {Path(__file__).resolve()} apply {out}")
    return 0


def cmd_apply(args: argparse.Namespace) -> int:
    out = Path(args.plan_dir)
    plan = json.loads((out / "plan.json").read_text())
    graph_path = out / "graph.json"
    result: dict = {"created": {}, "reopened": [], "closed": [], "updated": []}

    # Validate the graph before touching anything, so a bad plan changes nothing.
    if plan["graph"]["nodes"]:
        bd("create", "--graph", str(graph_path), "--dry-run")

    # Reopen before creating, so new children never land under a closed parent.
    # Epic before features, for the same reason.
    for tier in ("epic", "feature"):
        ids = [r["id"] for r in plan.get("reopens", []) if r["type"] == tier]
        if ids:
            bd("reopen", *ids, "--reason", "New open tasks in tasks.md")
            result["reopened"] += ids

    if plan["graph"]["nodes"]:
        created = json.loads(bd("create", "--graph", str(graph_path), "--json"))
        if "error" in created:
            raise SystemExit(f"bd create --graph failed: {created['error']}")
        result["created"] = created.get("ids", {})

    # bd will not close a parent that still has open children, so close bottom-up.
    for tier in ("task", "feature", "epic"):
        ids = [c["id"] for c in plan["closes"] if c["type"] == tier]
        if ids:
            bd("close", *ids, "--reason", "Checked off in tasks.md")
            result["closed"] += ids

    for u in plan["updates"]:
        args_ = ["update", u["id"]]
        if "title" in u:
            args_ += ["--title", u["title"]]
        if "description" in u:
            args_ += ["--description", u["description"]]
        bd(*args_)
        result["updated"].append(u["id"])

    (out / "result.json").write_text(json.dumps(result, indent=2) + "\n")
    print(f"Created {len(result['created'])}, reopened {len(result['reopened'])}, "
          f"closed {len(result['closed'])}, updated {len(result['updated'])}.")
    if result["created"].get("epic"):
        print(f"Epic: {result['created']['epic']}")
    print(f"Details: {out / 'result.json'}")
    return 0


def main(argv: list[str] | None = None) -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    sub = ap.add_subparsers(dest="cmd", required=True)

    p = sub.add_parser("plan", help="parse tasks.md and write a plan (no beads changes)")
    p.add_argument("tasks_md")
    p.add_argument("--out", help="plan directory (default: a new temp dir)")
    p.add_argument("--offline", action="store_true", help="skip the beads lookup; plan a fresh create")
    p.add_argument("--update-changed", action="store_true",
                   help="update titles/descriptions of existing beads that differ from tasks.md")
    p.add_argument("--priority", type=int, default=2, choices=range(5),
                   help="priority for the epic and phases without '(Priority: Pn)' (default 2)")
    p.set_defaults(func=cmd_plan)

    a = sub.add_parser("apply", help="execute a plan directory written by 'plan'")
    a.add_argument("plan_dir")
    a.set_defaults(func=cmd_apply)

    args = ap.parse_args(argv)
    return args.func(args)


if __name__ == "__main__":
    sys.exit(main())
