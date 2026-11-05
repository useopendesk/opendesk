import SwiftUI

/// Settings › AI agent › Overview.
struct AgentOverviewSettings: View {
    var body: some View {
        SettingsWebFallback(page: .agentOverview)
    }

    struct Detail: View {
        let id: String

        var body: some View {
            SettingsWebFallback(page: .agentOverview, id: id)
        }
    }
}
