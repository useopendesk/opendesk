import SwiftUI

/// Settings › AI agent › Playground.
struct PlaygroundSettings: View {
    var body: some View {
        SettingsWebFallback(page: .playground)
    }

    struct Detail: View {
        let id: String

        var body: some View {
            SettingsWebFallback(page: .playground, id: id)
        }
    }
}
