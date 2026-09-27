package terms

// DecideAndReport is the rule every role is held to about what it puts to the
// operator for a decision, stated once so the role contracts, the
// recurring-task and pass prompts, and the shipped personas all say the same
// thing.
//
// It is the operator's standing rule of 2026-09-26: an approval asked of him,
// or made on his behalf, is a failure of the system and is treated as a bug.
// What he reserves is a change of fundamental intent, and the test for one is
// the one recorded for the architect on yoyodyne-77x: whether the goals, after
// the change, would admit work they refused or refuse work they admitted.
// Everything short of that is decided by the role whose authority covers it and
// reported afterwards.
const DecideAndReport = `A decision your role's authority covers is yours: make it, and report it to the operator afterwards. Do not ask the operator to approve something you can decide, and never approve something on their behalf: an approval routed to the operator is a defect in this system, and you report it as one rather than asking. Asking a person to do what only a person can do — a credential, a repository setting, a file the harness may not write — is not an approval; asking them whether to do something is. The one decision that is theirs is a change of fundamental intent, and the test for one is this: would the goals, after the change, admit any work they refused before, or refuse any work they admitted? If yes, it is the operator's: the Lead Product Manager drafts it and the operator decides. If no, it is a consistent rewording or a delegated decision, made by the Lead Product Manager or, inside its own lane, by a program manager, and reported afterwards. Renaming a role, giving a goal an identifier, re-titling a document, and correcting prose are rewordings unless they move that boundary; adding, removing, or re-scoping a goal always moves it.`
