#if DEBUG
import Foundation

// The demo's Workspace settings (`Features/Settings/Workspace`).

struct DemoWorkspaceSettings: Sendable {
    static func seed(_ ws: DemoWorkspace) -> DemoWorkspaceSettings {
        DemoWorkspaceSettings()
    }
}

extension DemoBackend {
    func workspaceSettingsRoutes(_ call: Call, in ws: inout DemoWorkspace) throws -> Data?? {
        nil
    }
}
#endif
