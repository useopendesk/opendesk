import Foundation
import Observation

/// The ticket list for one folder or view, with filters, sort and search,
/// patched in place by realtime events.
@MainActor
@Observable
final class InboxStore {
    private(set) var scope: InboxScope = .folder(.mine)
    var filters = InboxFilters() {
        didSet { if filters != oldValue { reload() } }
    }
    var sort: TicketSort = .latest {
        didSet { if sort != oldValue { reload() } }
    }
    private(set) var search = ""
    private(set) var tickets: [Ticket] = []
    private(set) var nextCursor: String?
    private(set) var loading = false
    private(set) var loadingMore = false
    private(set) var error: String?
    private(set) var loadedOnce = false

    @ObservationIgnored private var api: APIClient?
    @ObservationIgnored private var me: String?
    @ObservationIgnored private var generation = 0
    @ObservationIgnored private var reloadTask: Task<Void, Never>?
    @ObservationIgnored private var searchTask: Task<Void, Never>?
    @ObservationIgnored private var instance: Instance?

    func start(api: APIClient, me: String) {
        self.api = api
        self.me = me
        instance = api.instance
        tickets = []
        nextCursor = nil
        loadedOnce = false
        error = nil
        if let raw = Preferences.lastFolder(for: api.instance), let saved = InboxScope(key: raw) {
            scope = saved
        } else {
            scope = .folder(.mine)
        }
        if let debug = AppInfo.debugValue("OPENDESK_DEBUG_FOLDER"), let folder = Folder(rawValue: debug) {
            scope = .folder(folder)
        }
        reload()
    }

    func stop() {
        generation += 1
        api = nil
        tickets = []
        nextCursor = nil
        loadedOnce = false
    }

    func setScope(_ next: InboxScope) {
        guard next != scope else { return }
        scope = next
        if let instance { Preferences.setLastFolder(next.key, for: instance) }
        tickets = []
        nextCursor = nil
        loadedOnce = false
        filters = InboxFilters()
        reload()
    }

    func setSearch(_ text: String) {
        let trimmed = text.trimmingCharacters(in: .whitespacesAndNewlines)
        guard trimmed != search else { return }
        search = trimmed
        searchTask?.cancel()
        searchTask = Task { [weak self] in
            try? await Task.sleep(for: .milliseconds(280))
            guard !Task.isCancelled else { return }
            self?.reload()
        }
    }

    private var query: [URLQueryItem] {
        var items: [URLQueryItem] = []
        switch scope {
        case let .folder(folder): items += folder.query
        case let .view(id): items.append(URLQueryItem(name: "view_id", value: id))
        }
        items += filters.query
        items += sort.query
        if !search.isEmpty { items.append(URLQueryItem(name: "q", value: search)) }
        items.append(URLQueryItem(name: "limit", value: "50"))
        return items
    }

    func reload() {
        reloadTask?.cancel()
        reloadTask = Task { await self.refresh() }
    }

    func refresh() async {
        guard let api else { return }
        generation += 1
        let current = generation
        loading = true
        defer { if current == generation { loading = false } }
        do {
            let page: Page<Ticket> = try await api.get("v1/tickets", query: query)
            guard current == generation else { return }
            tickets = page.data
            nextCursor = page.nextCursor
            error = nil
            loadedOnce = true
        } catch {
            guard current == generation, !error.isCancellation else { return }
            self.error = error.userMessage
            loadedOnce = true
        }
    }

    func loadMore() async {
        guard let api, let cursor = nextCursor, !loadingMore, !loading else { return }
        loadingMore = true
        defer { loadingMore = false }
        let current = generation
        var items = query
        items.append(URLQueryItem(name: "cursor", value: cursor))
        guard let page: Page<Ticket> = try? await api.get("v1/tickets", query: items), current == generation else { return }
        let known = Set(tickets.map(\.id))
        tickets += page.data.filter { !known.contains($0.id) }
        nextCursor = page.nextCursor
    }

    // MARK: Changes

    /// Applies a change at once and sends it; puts the ticket back if the server refuses.
    func patch(_ ticket: Ticket, _ change: TicketPatch, members: [Member], meRef: UserRef?) async throws -> Ticket {
        guard let api else { throw APIError.signedOut }
        let before = tickets
        let optimistic = change.applied(to: ticket, me: meRef, members: members)
        place(optimistic)
        do {
            let saved: Ticket = try await api.patch("v1/tickets/\(ticket.id)", body: change)
            place(saved)
            return saved
        } catch {
            tickets = before
            throw error
        }
    }

    /// Puts a fresh copy where it belongs: updated in place, inserted, or dropped when it left this list.
    func place(_ ticket: Ticket) {
        let belongs = belongsHere(ticket)
        if let index = tickets.firstIndex(where: { $0.id == ticket.id }) {
            if belongs == false {
                tickets.remove(at: index)
            } else {
                tickets[index] = ticket
                resort()
            }
        } else if belongs == true, search.isEmpty {
            tickets.append(ticket)
            resort()
        }
    }

    func remove(_ id: String) {
        tickets.removeAll { $0.id == id }
    }

    /// true / false when the scope can be checked here, nil for views (kept in place, the server decides on reload).
    private func belongsHere(_ ticket: Ticket) -> Bool? {
        switch scope {
        case let .folder(folder):
            return folder.contains(ticket, me: me) && filters.matches(ticket)
        case .view:
            return nil
        }
    }

    private func resort() {
        switch sort {
        case .latest: tickets.sort { $0.lastMessageAt > $1.lastMessageAt }
        case .oldest: tickets.sort { $0.lastMessageAt < $1.lastMessageAt }
        case .priority:
            tickets.sort { $0.priority.rank != $1.priority.rank ? $0.priority.rank > $1.priority.rank : $0.lastMessageAt > $1.lastMessageAt }
        case .due:
            func due(_ ticket: Ticket) -> Date { SLAState.of(ticket)?.due ?? .distantFuture }
            tickets.sort { due($0) < due($1) }
        }
    }

    // MARK: Realtime

    func handle(_ update: RealtimeUpdate) {
        switch update {
        case .resync:
            reload()
        case let .envelope(envelope):
            switch envelope.type {
            case "ticket.created", "ticket.updated":
                guard let ticket = envelope.member("ticket", as: Ticket.self) else { return }
                if case .view = scope, !tickets.contains(where: { $0.id == ticket.id }) {
                    scheduleQuietReload()
                } else {
                    place(ticket)
                }
            case "ticket.deleted":
                if let id = envelope.string("ticket_id") { remove(id) }
            case "tag.merged", "tag.updated", "tag.deleted":
                scheduleQuietReload()
            default:
                break
            }
        }
    }

    @ObservationIgnored private var quietReload: Task<Void, Never>?

    private func scheduleQuietReload() {
        guard quietReload == nil else { return }
        quietReload = Task { [weak self] in
            try? await Task.sleep(for: .milliseconds(600))
            guard let self else { return }
            self.quietReload = nil
            await self.refresh()
        }
    }
}
