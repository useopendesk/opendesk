#if DEBUG
import Foundation

// The demo's Inbox settings (`Features/Settings/Inbox`).

struct DemoInboxSettings: Sendable {
    static func seed(_ ws: DemoWorkspace) -> DemoInboxSettings {
        DemoInboxSettings()
    }
}

extension DemoBackend {
    func inboxSettingsRoutes(_ call: Call, in ws: inout DemoWorkspace) throws -> Data?? {
        nil
    }
}
#endif
