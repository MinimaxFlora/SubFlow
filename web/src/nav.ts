export interface NavItem {
  key: string
  label: string
  icon: string
}

// Primary destinations, rendered in the left sidebar (desktop) and the
// bottom tab bar (mobile), mirroring Sub-Store's navigation shell.
export const navItems: NavItem[] = [
  { key: 'overview', label: 'Overview', icon: 'gauge' },
  { key: 'subs', label: 'Subscriptions', icon: 'layers' },
  { key: 'convert', label: 'Converter', icon: 'convert' },
  { key: 'settings', label: 'Settings', icon: 'sliders' },
]
