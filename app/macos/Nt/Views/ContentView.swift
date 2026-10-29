import SwiftUI

/// ContentView is the window: the sidebar beside the note once the repository is found, or why
/// it is not.
struct ContentView: View {
    @Environment(Workspace.self) private var ws

    var body: some View {
        Group {
            switch ws.phase {
            case .loading:
                ProgressView()
                    .controlSize(.small)
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
            case .problem(let message):
                ProblemView(message: message)
            case .ready:
                NavigationSplitView(columnVisibility: sidebar) {
                    SidebarView()
                        .navigationSplitViewColumnWidth(min: 200, ideal: 248, max: 380)
                } detail: {
                    switch ws.selection {
                    case .todos: TodosView()
                    case .calendar: CalendarView()
                    case .agents: AgentsView()
                    case .weekly where ws.weeklyPath == nil: WeeklyEmptyView()
                    default: NoteView()
                    }
                }
            }
        }
        .background(Brand.ground)
        .modifier(Asking())
        .overlay {
            if let mode = ws.palette {
                PaletteView(mode: mode)
            }
        }
    }
}

extension ContentView {
    private var sidebar: Binding<NavigationSplitViewVisibility> {
        Binding(get: { ws.sidebar ? .all : .detailOnly }, set: { ws.sidebar = $0 != .detailOnly })
    }
}

/// ProblemView says what stops the app and how to fix it.
struct ProblemView: View {
    let message: String
    @Environment(Workspace.self) private var ws

    var body: some View {
        VStack(spacing: 14) {
            Text("[≡▁] nt")
                .font(Brand.mono(22, bold: true))
                .foregroundStyle(Brand.accent)
            Text(message)
                .font(.system(size: 14))
                .foregroundStyle(Brand.ink)
                .multilineTextAlignment(.center)
                .textSelection(.enabled)
                .frame(maxWidth: 460)
            Button("Try again") { Task { await ws.start() } }
                .keyboardShortcut(.defaultAction)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }
}
