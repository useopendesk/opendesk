import SwiftUI

/// Settings › AI agent › Knowledge.
struct KnowledgeSettings: View {
    var body: some View {
        SettingsWebFallback(page: .knowledge)
    }

    struct Detail: View {
        let id: String

        var body: some View {
            SettingsWebFallback(page: .knowledge, id: id)
        }
    }
}
