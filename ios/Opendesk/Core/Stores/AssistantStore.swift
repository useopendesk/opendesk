import Foundation
import Observation

/// A conversation with Ollie: streamed answers, tool steps, and approvals
/// for anything that would change data (contracts/ai.md "Ollie").
@MainActor
@Observable
final class AssistantStore {
    struct Turn: Identifiable, Hashable {
        var id = UUID()
        var role: String
        var text: String
        var steps: [AssistantStep] = []
    }

    private(set) var session: AssistantSession?
    private(set) var turns: [Turn] = []
    private(set) var approval: AssistantApproval?
    private(set) var running = false
    private(set) var deciding = false
    private(set) var error: String?

    @ObservationIgnored private let api: APIClient
    @ObservationIgnored private var runTask: Task<Void, Never>?

    init(api: APIClient) {
        self.api = api
    }

    func reset() {
        runTask?.cancel()
        session = nil
        turns = []
        approval = nil
        running = false
        error = nil
    }

    func send(_ text: String, context: AssistantPageContext?) {
        let trimmed = text.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty, !running else { return }
        turns.append(Turn(role: "user", text: trimmed))
        approval = nil
        error = nil
        runTask = Task { await self.run(text: trimmed, context: context) }
    }

    func stop() {
        runTask?.cancel()
        running = false
    }

    private func run(text: String, context: AssistantPageContext?) async {
        struct Body: Encodable, Sendable {
            let text: String
            let context: AssistantPageContext?
        }
        running = true
        defer { running = false }
        do {
            if session == nil {
                struct NewSession: Encodable, Sendable {}
                session = try await api.post("v1/assistant/sessions", body: NewSession())
            }
            guard let session else { return }
            let stream = try await api.stream("v1/assistant/sessions/\(session.id)/messages", body: Body(text: text, context: context))
            try await consume(stream)
        } catch {
            if !error.isCancellation { self.error = error.userMessage }
        }
    }

    func decide(approve: Bool) {
        guard let approval, !deciding else { return }
        struct Body: Encodable, Sendable {
            let decision: String
            let token: String
        }
        deciding = true
        error = nil
        runTask = Task {
            running = true
            defer {
                running = false
                deciding = false
            }
            do {
                let stream = try await api.stream("v1/assistant/actions/\(approval.actionID)/decision", body: Body(decision: approve ? "approve" : "deny", token: approval.token))
                self.approval = nil
                try await consume(stream)
            } catch {
                if !error.isCancellation { self.error = error.userMessage }
            }
        }
    }

    private func consume(_ stream: AsyncThrowingStream<SSEEvent, Error>) async throws {
        var answerIndex: Int?
        func answer() -> Int {
            if let answerIndex { return answerIndex }
            turns.append(Turn(role: "assistant", text: ""))
            answerIndex = turns.count - 1
            return turns.count - 1
        }
        for try await event in stream {
            guard let json = event.json() else { continue }
            switch event.event {
            case "session":
                if let value = try? json.decode(AssistantSession.self) { session = value }
            case "text":
                let index = answer()
                turns[index].text += json["text"]?.string ?? ""
            case "tool":
                guard let step = try? json.decode(AssistantStep.self) else { continue }
                let index = answer()
                if let existing = turns[index].steps.firstIndex(where: { $0.id == step.id }) {
                    turns[index].steps[existing] = step
                } else {
                    turns[index].steps.append(step)
                }
            case "approval":
                approval = try? json.decode(AssistantApproval.self)
            case "error":
                error = APIError.parse(status: json["status"]?.int ?? 500, data: Data(event.data.utf8)).userMessage
            case "result":
                // The server answered in one piece.
                guard let result = try? json.decode(AssistantRunResult.self) else { continue }
                if let value = result.session { session = value }
                let index = answer()
                turns[index].text = result.text ?? ""
                turns[index].steps = result.steps ?? []
                approval = result.approval
            default:
                break
            }
        }
    }
}
