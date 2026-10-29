// Shortcuts are the app's two App Intents (docs/design.md §9, Capture): "Add to today" and "New
// todo", for Shortcuts, Siri, Spotlight and the Action button. They write as the capture sheet
// does, through nt in the app's process, without opening the app; then the phone syncs.
import AppIntents

/// AddToToday adds a line to today's journal, under Log, as nt a does.
struct AddToToday: AppIntent {
    static let title: LocalizedStringResource = "Add to today"
    static let description = IntentDescription("Adds a line to today's journal, under Log.")

    @Parameter(title: "Line", requestValueDialog: "What should be added to today?")
    var text: String

    @Dependency private var phone: Phone

    static var parameterSummary: some ParameterSummary {
        Summary("Add \(\.$text) to today")
    }

    @MainActor
    func perform() async throws -> some IntentResult & ProvidesDialog {
        .result(dialog: "\(try await phone.added(text, todo: false))")
    }
}

/// NewTodo adds a todo to the end of Inbox in the root's todo.md, as nt todo add does; due:fri in
/// the text sets its date.
struct NewTodo: AppIntent {
    static let title: LocalizedStringResource = "New todo"
    static let description = IntentDescription("Adds a todo to Inbox in todo.md. due:fri in the text sets its due date.")

    @Parameter(title: "Todo", requestValueDialog: "What is the todo?")
    var text: String

    @Dependency private var phone: Phone

    static var parameterSummary: some ParameterSummary {
        Summary("New todo \(\.$text)")
    }

    @MainActor
    func perform() async throws -> some IntentResult & ProvidesDialog {
        .result(dialog: "\(try await phone.added(text, todo: true))")
    }
}

/// NtShortcuts offers both to Siri, Spotlight and the Action button without setting anything up.
struct NtShortcuts: AppShortcutsProvider {
    static var appShortcuts: [AppShortcut] {
        AppShortcut(
            intent: AddToToday(),
            phrases: ["Add to today in \(.applicationName)", "Add to today's \(.applicationName) journal"],
            shortTitle: "Add to today",
            systemImageName: "square.and.pencil"
        )
        AppShortcut(
            intent: NewTodo(),
            phrases: ["New todo in \(.applicationName)", "Add a todo in \(.applicationName)", "New \(.applicationName) todo"],
            shortTitle: "New todo",
            systemImageName: "circle"
        )
    }
}

/// Refusal is why a Shortcut added nothing, in nt's words, for Siri or Shortcuts to say.
struct Refusal: Error, CustomLocalizedStringResourceConvertible {
    let why: String
    var localizedStringResource: LocalizedStringResource { "\(why)" }
}

extension Phone {
    /// added adds text for a Shortcut, syncs it, and says where it went.
    fileprivate func added(_ text: String, todo: Bool) async throws -> String {
        do {
            let a = try await add(text, todo: todo)
            syncAdded()
            return a.said
        } catch {
            throw Refusal(why: error.localizedDescription)
        }
    }
}
