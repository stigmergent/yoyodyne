package terms

// LiveCopy is the rule a run is held to when its deliverable is how a role
// behaves: the change is done when the copy the role reads carries it, stated
// once so the developer contract and the reviewer's contract say the same thing.
//
// On 2026-09-27 the rule against routing approvals to the operator
// (yoyodyne-ifd.430.21) and the rule for naming work items by what they are
// (yoyodyne-ifd.430.22) landed in the personas the executable ships as its
// template and closed as done, while the persona files this project's roles
// actually read under .yoyodyne/personas carried neither. Every role ran
// without them while the tracker recorded them delivered, because nothing
// asked which copy a change had reached (yoyodyne-ifd.430.26).
const LiveCopy = `A change whose deliverable is how a role behaves is done when the copy that role reads carries it, and not before. A role reads the persona file the project's configuration binds for it, under .yoyodyne/personas, and the contract compiled into the harness; the personas the executable ships under internal/config/builtin are the template "yoyo init" gives a new project, and no running role reads them. So a persona change that lands in the template alone has changed nothing any role does. Make it in the bound copy too, which needs that path granted by the work item; where the item grants none, the item is not discharged by your change: land it as evidence, say in the landing's reason which bound copy does not carry the change, and name the gap "yoyo config drift" reports, or the files themselves where it reports none.`
