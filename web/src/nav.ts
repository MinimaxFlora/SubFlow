export interface NavItem {
  key: string
  labelKey: string
  icon: string
}

// Primary destinations, rendered in the left sidebar (desktop) and the
// bottom tab bar (mobile), mirroring Sub-Store's navigation shell.
export const navItems: NavItem[] = [
  { key: 'overview', labelKey: 'nav.overview', icon: 'gauge' },
  { key: 'subs', labelKey: 'nav.subs', icon: 'layers' },
  { key: 'convert', labelKey: 'nav.convert', icon: 'convert' },
  { key: 'settings', labelKey: 'nav.settings', icon: 'sliders' },
]
