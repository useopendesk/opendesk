import SwiftUI

/// Settings › AI.
struct AISettings: View {
    var body: some View {
        SettingsWebFallback(page: .ai)
    }

    struct Detail: View {
        let id: String

        var body: some View {
            SettingsWebFallback(page: .ai, id: id)
        }
    }
}
