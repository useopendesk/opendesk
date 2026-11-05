import SwiftUI

/// Settings › API keys.
struct APIKeysSettings: View {
    var body: some View {
        SettingsWebFallback(page: .apiKeys)
    }

    struct Detail: View {
        let id: String

        var body: some View {
            SettingsWebFallback(page: .apiKeys, id: id)
        }
    }
}
