---
id: markdown-source-of-truth
kind: decision
title: Repository Markdown is the human-readable source of truth
status: active
revisions:
    - action: created
      by: architect
      at: 2026-08-17T00:00:00Z
      reason: identity added with the artifact metadata schema; the record itself is unchanged
    - action: amended
      by: architect
      at: 2026-08-19T00:00:00Z
      reason: drafting residue removed — a stray code fence and trailing rule, and the offer of ratification the architect has since given; the decision itself is unchanged
    - action: amended
      by: architect
      at: 2026-09-29T02:00:00Z
      reason: yoyodyne-ifd.78 - the repository holding the intent is the project's own by default or a companion intent repository under the machine home, chosen per project; what the documents are does not change
---

# Repository Markdown is the human-readable source of truth

**Status:** Accepted. This decision was made and acted on during the v1 design;
it was stated in [the v1 goals](../product/goals/v1-goals.md) until it was
moved here, because it is a decision about how Yoyodyne is built rather than
an outcome the product is trying to reach.

## Context

The traceable chain from brief to merged change is only useful if a person can
read it. It has to be reviewable by the same means as the code it governs,
diffable, versioned alongside the changes it justifies, and legible without
running Yoyodyne at all.

## Decision

The brief, goals, designs, specifications, decision records, and invariants are
Markdown files in a git repository the harness reads at a recorded commit. By
default that is the project's own repository, versioned beside the code they
govern. A project may instead keep them, with the tracker's export, in a
companion intent repository: a git repository of their own that the harness
holds under its machine home and that is pushed and cloned like any other, so
that a project built by Yoyodyne need carry nothing of Yoyodyne's in its own
repository. Which repository holds them is machine-local configuration, and it
changes nothing about what they are: the source of truth for artifact content,
read at a recorded commit, owned by role, revised through a log. Any
machine-readable metadata those documents carry travels with them in
the repository rather than in a separate store.

## Consequences

- Artifact content is versioned and reviewed through the same Git history and the
  same pull requests as code.
- Ownership of a document is an authorization boundary rather than a convention:
  a role that does not own a document proposes a change to it instead of writing
  one.
- A document's structure can be checked. That checking must distinguish documents
  that are expected to state goals from index and non-goals documents; see
  yoyodyne-ifd.43.
- Yoyodyne remains readable and auditable by someone who has never run it, which
  is what the product's traceability goal ultimately rests on.
- Where a companion intent repository is used, a change is evidenced by two
  commits, the code's and the intent's, and every check that resolves a
  governed document by path resolves it there;
  [the machine home](../designs/machine-home.md) says how. What the decision
  protects is unchanged: a person reads the companion repository exactly as
  they read this one.
