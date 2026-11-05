import SwiftUI

/// Settings › Automations.
struct AutomationsSettings: View {
    var body: some View {
        SettingsWebFallback(page: .automations)
    }

    struct Detail: View {
        let id: String

        var body: some View {
            SettingsWebFallback(page: .automations, id: id)
        }
    }
}
