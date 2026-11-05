import SwiftUI

/// Settings › AI agent › Tools.
struct ToolsSettings: View {
    var body: some View {
        SettingsWebFallback(page: .tools)
    }

    struct Detail: View {
        let id: String

        var body: some View {
            SettingsWebFallback(page: .tools, id: id)
        }
    }
}
