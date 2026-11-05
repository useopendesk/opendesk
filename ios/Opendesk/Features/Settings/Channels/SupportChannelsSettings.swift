import SwiftUI

/// Settings › Support channels.
struct SupportChannelsSettings: View {
    var body: some View {
        SettingsWebFallback(page: .supportChannels)
    }

    struct Detail: View {
        let id: String

        var body: some View {
            SettingsWebFallback(page: .supportChannels, id: id)
        }
    }
}
