import { useEffect, useState } from 'react'
import { BookOpen, Database, FileText, Gauge, LayoutDashboard, Map, Plus, Settings, type LucideIcon } from 'lucide-react'

const BASE_PATH = import.meta.env.BASE_URL.replace(/\/$/, '')

type SectionId = 'overview' | 'intake' | 'reports' | 'saved-exams' | 'protocol' | 'settings' | 'benchmark' | 'roadmap'

export interface Section {
  id: SectionId
  label: string
  path: string
  icon: LucideIcon
  group: 'PLATAFORMA' | 'SISTEMA'
}

export const sections: Section[] = [
  { id: 'overview', label: 'Visão geral', path: 'visao-geral', icon: LayoutDashboard, group: 'PLATAFORMA' },
  { id: 'intake', label: 'Nova análise', path: 'nova-analise', icon: Plus, group: 'PLATAFORMA' },
  { id: 'reports', label: 'Relatórios', path: 'relatorios', icon: FileText, group: 'PLATAFORMA' },
  { id: 'saved-exams', label: 'Exames salvos', path: 'exames-salvos', icon: Database, group: 'PLATAFORMA' },
  { id: 'protocol', label: 'Protocolo', path: 'protocolo', icon: BookOpen, group: 'PLATAFORMA' },
  { id: 'settings', label: 'Configurações', path: 'configuracoes', icon: Settings, group: 'SISTEMA' },
  { id: 'benchmark', label: 'Benchmark OCR', path: 'benchmark-ocr', icon: Gauge, group: 'SISTEMA' },
  { id: 'roadmap', label: 'Roadmap', path: 'roadmap', icon: Map, group: 'SISTEMA' },
]

const sectionById = (id: SectionId) => sections.find((section) => section.id === id)!
const REPORTS_PATH = sectionById('reports').path

function parseRoute(pathname: string) {
  const relative = (pathname.startsWith(BASE_PATH) ? pathname.slice(BASE_PATH.length) : pathname).replace(/^\/|\/$/g, '')
  const reportId = relative.match(new RegExp(`^${REPORTS_PATH}/([^/]+)$`))?.[1] ?? null
  const section = reportId ? sectionById('reports') : sections.find((item) => item.path === relative) ?? sectionById('intake')
  return { section, reportId }
}

export function useRoute() {
  const [pathname, setPathname] = useState(() => window.location.pathname)

  useEffect(() => {
    const onPopState = () => setPathname(window.location.pathname)
    window.addEventListener('popstate', onPopState)
    return () => window.removeEventListener('popstate', onPopState)
  }, [])

  function navigate(path: string) {
    window.history.pushState({}, '', `${BASE_PATH}/${path}`)
    setPathname(window.location.pathname)
  }

  return {
    ...parseRoute(pathname),
    goTo: (id: SectionId) => navigate(sectionById(id).path),
    openReport: (reportId: string) => navigate(`${REPORTS_PATH}/${reportId}`),
  }
}
