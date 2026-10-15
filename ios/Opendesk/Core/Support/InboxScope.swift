import Foundation

/// The inbox folders, as on the web (`web/src/lib/folders.ts`).
enum Folder: String, CaseIterable, Identifiable, Sendable, Codable {
    case unassigned, mine, open, pending, solved, spam

    var id: String { rawValue }

    var title: String {
        switch self {
        case .unassigned: "Unassigned"
        case .mine: "Mine"
        case .open: "All open"
        case .pending: "Pending"
        case .solved: "Solved"
        case .spam: "Spam"
        }
    }

    var symbol: String {
        switch self {
        case .unassigned: "tray"
        case .mine: "person.crop.circle"
        case .open: "circle"
        case .pending: "clock"
        case .solved: "checkmark.circle"
        case .spam: "xmark.octagon"
        }
    }

    var query: [URLQueryItem] {
        switch self {
        case .unassigned: [.init(name: "status", value: "open,pending,on_hold"), .init(name: "assignee", value: "none")]
        case .mine: [.init(name: "status", value: "open,pending,on_hold"), .init(name: "assignee", value: "me")]
        case .open: [.init(name: "status", value: "open")]
        case .pending: [.init(name: "status", value: "pending,on_hold")]
        case .solved: [.init(name: "status", value: "solved,closed")]
        case .spam: [.init(name: "spam", value: "true")]
        }
    }

    func count(in counts: TicketCounts?) -> Int? {
        guard let counts else { return nil }
        switch self {
        case .unassigned: return counts.unassigned
        case .mine: return counts.mine
        case .open: return counts.open
        case .pending: return counts.pending + counts.onHold
        case .solved: return nil
        case .spam: return counts.spam
        }
    }

    /// Whether a ticket belongs here, to keep a loaded list right as events arrive.
    func contains(_ ticket: Ticket, me: String?) -> Bool {
        if self == .spam { return ticket.spam }
        if ticket.spam { return false }
        switch self {
        case .unassigned: return [.open, .pending, .onHold].contains(ticket.status) && ticket.assignee == nil
        case .mine: return [.open, .pending, .onHold].contains(ticket.status) && ticket.assignee?.id == me && me != nil
        case .open: return ticket.status == .open
        case .pending: return ticket.status == .pending || ticket.status == .onHold
        case .solved: return ticket.status.isDone
        case .spam: return ticket.spam
        }
    }
}

/// What the inbox list shows: a folder or a saved view.
enum InboxScope: Hashable, Sendable {
    case folder(Folder)
    case view(String)

    var key: String {
        switch self {
        case let .folder(folder): "folder:" + folder.rawValue
        case let .view(id): "view:" + id
        }
    }

    init?(key: String) {
        if key.hasPrefix("folder:"), let folder = Folder(rawValue: String(key.dropFirst(7))) {
            self = .folder(folder)
        } else if key.hasPrefix("view:") {
            self = .view(String(key.dropFirst(5)))
        } else {
            return nil
        }
    }

    /// The web path of a ticket in this scope (`/inbox/<folder>/<id>`, `/views/<view>/<id>`).
    func webPath(ticketID: String? = nil) -> String {
        let base: String
        switch self {
        case let .folder(folder): base = "/inbox/\(folder.rawValue)"
        case let .view(id): base = "/views/\(id)"
        }
        guard let ticketID else { return base }
        return base + "/" + ticketID
    }
}

/// Sort orders the list offers.
enum TicketSort: String, CaseIterable, Identifiable, Sendable {
    case latest, oldest, priority, due

    var id: String { rawValue }

    var title: String {
        switch self {
        case .latest: "Newest activity"
        case .oldest: "Oldest activity"
        case .priority: "Priority"
        case .due: "Due soonest"
        }
    }

    var query: [URLQueryItem] {
        switch self {
        case .latest: [.init(name: "sort", value: "last_message_at"), .init(name: "order", value: "desc")]
        case .oldest: [.init(name: "sort", value: "last_message_at"), .init(name: "order", value: "asc")]
        case .priority: [.init(name: "sort", value: "priority"), .init(name: "order", value: "desc")]
        case .due: [.init(name: "sort", value: "due"), .init(name: "order", value: "asc")]
        }
    }
}

/// Extra filters on top of a folder or view.
struct InboxFilters: Hashable, Sendable {
    var priorities: Set<Priority> = []
    var channel: Channel?
    var teamID: String?
    var tag: String?

    var isEmpty: Bool { priorities.isEmpty && channel == nil && teamID == nil && tag == nil }
    var count: Int { (priorities.isEmpty ? 0 : 1) + (channel == nil ? 0 : 1) + (teamID == nil ? 0 : 1) + (tag == nil ? 0 : 1) }

    var query: [URLQueryItem] {
        var items: [URLQueryItem] = []
        if !priorities.isEmpty {
            items.append(.init(name: "priority", value: Priority.allCases.filter(priorities.contains).map(\.rawValue).joined(separator: ",")))
        }
        if let channel { items.append(.init(name: "channel", value: channel.rawValue)) }
        if let teamID { items.append(.init(name: "team_id", value: teamID)) }
        if let tag { items.append(.init(name: "tag", value: tag)) }
        return items
    }

    func matches(_ ticket: Ticket) -> Bool {
        if !priorities.isEmpty, !priorities.contains(ticket.priority) { return false }
        if let channel, ticket.channel != channel { return false }
        if let teamID, ticket.teamID != teamID { return false }
        if let tag, !ticket.tags.contains(where: { $0.name.caseInsensitiveCompare(tag) == .orderedSame }) { return false }
        return true
    }
}
