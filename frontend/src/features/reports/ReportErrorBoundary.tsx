import { Component, type ReactNode } from 'react'
import { Eyebrow } from '../../shared/ui'

export default class ReportErrorBoundary extends Component<{ children: ReactNode; onBack: () => void }, { error: Error | null }> {
  state: { error: Error | null } = { error: null }

  static getDerivedStateFromError(error: Error) {
    return { error }
  }

  render() {
    if (!this.state.error) return this.props.children
    return (
      <section className="mt-5 w-full max-w-xl rounded-2xl border border-danger bg-surface p-6 shadow-sm">
        <Eyebrow>ERRO AO CARREGAR RELATÓRIO</Eyebrow>
        <h2 className="mb-0 mt-2 font-display text-2xl">Não foi possível exibir este caso</h2>
        <p className="mb-0 mt-3 text-sm leading-relaxed text-text-secondary">O JSON salvo não contém todos os campos necessários para este fluxo.</p>
        <p className="mb-0 mt-3 text-xs text-text-muted">{this.state.error.message}</p>
        <button className="mt-5 rounded-[9px] bg-primary px-4 py-2.5 text-sm font-semibold text-white" onClick={this.props.onBack} type="button">Voltar para casos</button>
      </section>
    )
  }
}
