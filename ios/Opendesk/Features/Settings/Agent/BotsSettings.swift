import SwiftUI

/// Settings › AI agent › Bots.
struct BotsSettings: View {
    var body: some View {
        SettingsWebFallback(page: .bots)
    }

    struct Detail: View {
        let id: String

        var body: some View {
            SettingsWebFallback(page: .bots, id: id)
        }
    }
}
