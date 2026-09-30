---
id: who-approves-a-change-to-intent
kind: decision
title: Fundamental intent is the operator's; everything consistent with it is delegated, on one test
status: active
revisions:
    - action: created
      by: architect
      at: 2026-09-27T09:00:00Z
      reason: yoyodyne-77x - the operator's 2026-09-26 decision on who approves a change to intent, recorded with the test the Lead Product Manager stated for applying it, and the rulings and descriptions it amends named
---

# Fundamental intent is the operator's; everything consistent with it is delegated, on one test

**The operator's decision, 2026-09-26, in his words:** "If this is about who approves changes to the fundamental goals of the system, those should come from me. Anything else, as long as it remains consistent with those goals, can be delegated to the Lead PM or, if it's in their lane, the program managers." It bounds his standing rule of the same day that an approval routed to him is a defect: a change to what the product is for still comes from him, and everything else does not.

**Decision.**

1. A change to fundamental intent — what the product is for, who it is for, or what finished looks like — is the operator's. The Lead Product Manager drafts it, the operator decides it, and no role applies it before he has.
2. Every other change is delegated, provided it stays consistent with the goals: to the Lead Product Manager, or to a program manager where the change falls inside its lane. Routing such a change to the operator for approval is a defect under his rule.
3. The test, applied the same way by every role: **would the goals, after the change, admit any work they refused before, or refuse any work they admitted?** If yes, it is a change of fundamental intent. If no, it is a consistent rewording or a delegated decision: every item attributed to a goal before is attributed to the same goal after with its meaning unchanged, and a reader of the old and new text would admit and refuse the same work. Renaming a role, giving a goal an identifier, retitling a document, and correcting prose are rewordings unless they move that boundary; adding a goal, removing one, or moving what a goal covers always moves it. Applying the goals to a case decides nothing about the goals and is delegated by construction.
4. Settled under it: the Lead Product Manager rename (yoyodyne-ifd.437.11) and the goal identifiers (yoyodyne-ifd.344) are rewordings and the Lead Product Manager's to carry; a change to the approval wording of the brief or the autonomy goal that says the above is a change to how the goals are governed and is the operator's, once.

**What this amends.** The harness design's statement that the human explicitly approves the brief and goals is refined: he approves fundamental intent, and a consistent rewording is the owning role's revision. The "every decision is yours" arrangement for amendments is replaced by the owning role deciding, per the configurable-workflows authority model as amended for yoyodyne-ifd.437.14. And the artifact contract gains a revision action, `reworded`, for a consistent rewording: like `identified`, it leaves the operator's approval of the document standing, so a rewording the Lead Product Manager carries does not suspend automatic admission until he re-approves — which would route the approval back to him by another door. An `amended` revision is a change that moved the boundary, and resets the approval as it does today.

**Consequences for other documents.** The Lead Product Manager's contract and persona apply the test before proposing a change to the goals; `yoyo amendment`'s help stops saying every decision is the operator's; the artifact contract gains `reworded`; the follow-up work is the Lead Product Manager's to admit.
