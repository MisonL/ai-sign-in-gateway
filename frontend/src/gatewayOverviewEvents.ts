const GATEWAY_OVERVIEW_CHANGED_EVENT = 'gateway-overview-changed'

export function notifyGatewayOverviewChanged(): void {
  window.dispatchEvent(new Event(GATEWAY_OVERVIEW_CHANGED_EVENT))
}

export function onGatewayOverviewChanged(callback: () => void): () => void {
  const listener = () => callback()
  window.addEventListener(GATEWAY_OVERVIEW_CHANGED_EVENT, listener)
  return () => {
    window.removeEventListener(GATEWAY_OVERVIEW_CHANGED_EVENT, listener)
  }
}
