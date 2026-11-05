#if DEBUG
import Foundation

// The demo's Channels settings (`Features/Settings/Channels`).

struct DemoChannelSettings: Sendable {
    static func seed(_ ws: DemoWorkspace) -> DemoChannelSettings {
        DemoChannelSettings()
    }
}

extension DemoBackend {
    func channelSettingsRoutes(_ call: Call, in ws: inout DemoWorkspace) throws -> Data?? {
        nil
    }
}
#endif
