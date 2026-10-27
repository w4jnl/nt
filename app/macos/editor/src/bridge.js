// post sends a message to the app (WKScriptMessageHandler "nt"); outside the app it does nothing.
export function post(msg) {
  globalThis.webkit?.messageHandlers?.nt?.postMessage(msg);
}
