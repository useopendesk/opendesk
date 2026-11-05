#if DEBUG
import Foundation

// The demo's Developers and Admin settings (`Features/Settings/Developers`).

struct DemoDeveloperSettings: Sendable {
    static func seed(_ ws: DemoWorkspace) -> DemoDeveloperSettings {
        DemoDeveloperSettings()
    }
}

extension DemoBackend {
    func developerSettingsRoutes(_ call: Call, in ws: inout DemoWorkspace) throws -> Data?? {
        nil
    }
}
#endif
