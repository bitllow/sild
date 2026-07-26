// `import Sild` alone must put the whole SDK in scope. Without this, a host that writes
// only `import Sild` gets "cannot find 'SildConfig' in scope", because the shared types
// live in the SildCore binary module.
//
// `@_exported` is underscored, but SwiftPM has no supported way to re-export a target's
// dependency: `exportedDependencies` does not exist, and a `public typealias` per type
// would have to be maintained by hand for every model, enum and protocol the core
// exposes — and would still not carry enum cases or extensions. This is one line, and
// the alternative is a second import consumers must remember forever.
@_exported import SildCore
