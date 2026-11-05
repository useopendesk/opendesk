import SwiftUI

/// Settings › Agent-to-agent (Your settings).
struct AgentToAgentSettings: View {
    var body: some View {
        SettingsWebFallback(page: .agentToAgent)
    }

    struct Detail: View {
        let id: String

        var body: some View {
            SettingsWebFallback(page: .agentToAgent, id: id)
        }
    }
}
