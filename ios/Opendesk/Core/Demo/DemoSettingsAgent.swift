#if DEBUG
import Foundation

// The demo's AI agent settings (`Features/Settings/Agent`).

struct DemoAgentSettings: Sendable {
    static func seed(_ ws: DemoWorkspace) -> DemoAgentSettings {
        DemoAgentSettings()
    }
}

extension DemoBackend {
    func agentSettingsRoutes(_ call: Call, in ws: inout DemoWorkspace) throws -> Data?? {
        nil
    }
}
#endif
