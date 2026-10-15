import Foundation

/// The `od` member of a push (contracts/native.md "Sending").
struct PushPayload: Equatable, Sendable {
    var version: Int
    var server: String?
    var workspaceID: String?
    var notificationID: String?
    var ticketID: String?
    var ticketNumber: Int?
    var type: String?

    init?(userInfo: [AnyHashable: Any]) {
        guard let od = userInfo["od"] as? [String: Any] else { return nil }
        version = (od["v"] as? Int) ?? (od["v"] as? NSNumber)?.intValue ?? 1
        server = od["server"] as? String
        workspaceID = od["workspace_id"] as? String
        notificationID = od["notification_id"] as? String
        ticketID = od["ticket_id"] as? String
        ticketNumber = (od["ticket_number"] as? Int) ?? (od["ticket_number"] as? NSNumber)?.intValue
        type = od["type"] as? String
    }

    var kind: NotificationKind? { type.flatMap(NotificationKind.init(rawValue:)) }

    /// Where a tap goes: the ticket, or the notifications list for workspace alerts.
    var target: DeepLink.Target {
        let link: DeepLink = ticketID.map { .ticket(id: $0, scope: nil) } ?? .notifications
        return DeepLink.Target(link: link, server: server, workspaceID: workspaceID)
    }

    /// Whether the push came from this server. Compared by origin, so a trailing slash or path never matters.
    func isFrom(_ instance: Instance) -> Bool {
        guard let server, let url = URL(string: server) else { return true }
        return Self.origin(url) == Self.origin(instance.apiURL)
    }

    static func origin(_ url: URL) -> String {
        var text = (url.scheme ?? "https") + "://" + (url.host() ?? "").lowercased()
        if let port = url.port { text += ":\(port)" }
        return text
    }
}

/// Category and action identifiers registered with the system.
enum PushCategory {
    static let ticket = "TICKET"
    static let assignToMe = "ASSIGN_ME"
    static let markRead = "MARK_READ"
    static let reply = "REPLY"
}
