import SwiftUI
struct TicketView: View { let route: TicketRoute; var body: some View { Text(route.id) } }
struct NotificationsView: View { var body: some View { Text("n") } }
struct AccountView: View { var body: some View { Text("a") } }
struct AssistantView: View { let context: AssistantPageContext; var body: some View { Text("o") } }
