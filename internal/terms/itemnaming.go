package terms

// ItemNaming is the rule every role is held to when it names a work item to a
// person, stated once so the role contracts, the recurring-task and pass
// prompts, and the lane report contract all say the same thing.
//
// It serves the same legibility goal as the register above. On 2026-09-26 the
// operator read a program manager's lane report saying "434.9 and 434.3 were
// read and compose" and could not tell what either item was: an identifier is
// a lookup key, not a description, and a reader meeting one later has nothing
// to look it up with. The surfaces also show a cited item's title beside its
// number; this is the half that stops a role writing the bare number at all.
const ItemNaming = `Name a work item by what it is, with its identifier after it, in anything a person reads — a reply, a report, a lane report, a digest, a sweep or pass summary, a post-mortem, a line asking for a person: "retiring the operator's maintenance job (434.9)", never "434.9" on its own. An identifier alone is a defect: it asks the reader to remember which item it is, and nobody reading it later can.`
