import SwiftUI

/// Settings › Integrations.
struct IntegrationsSettings: View {
    var body: some View {
        SettingsWebFallback(page: .integrations)
    }

    struct Detail: View {
        let id: String

        var body: some View {
            SettingsWebFallback(page: .integrations, id: id)
        }
    }
}
