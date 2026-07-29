import { useState } from 'react'
import clsx from 'clsx'
import {
  Check,
  ChevronDown,
  Circle,
  Compass,
  Flag,
  Route,
  ShieldCheck,
  Sparkles,
} from 'lucide-react'

type RoadmapItem = {
  label: string
  completed: boolean
}

type RoadmapPhase = {
  id: string
  number: number
  title: string
  description: string
  status: 'Em andamento' | 'Próxima etapa' | 'Mais adiante'
  badge: 'Estamos aqui' | 'Vem a seguir' | 'Etapas futuras' | 'Concluído'
  items: RoadmapItem[]
}

const phases: RoadmapPhase[] = [
  {
    id: 'testes-com-medicos',
    number: 1,
    title: 'Testes de uso com médicos',
    description:
        'Validar como o médico envia os exames, confere as informações e entende o relatório.',
    status: 'Em andamento',
    badge: 'Estamos aqui',
    items: [
      {
        label: 'Estrutura inicial da plataforma',
        completed: true,
      },
      {
        label: 'Envio demonstrativo de exames',
        completed: true,
      },
      {
        label: 'Primeira versão do relatório',
        completed: true,
      },
      {
        label: 'Ajustes visuais da interface',
        completed: true,
      },
      {
        label: 'Conferência dos documentos enviados',
        completed: false,
      },
      {
        label: 'Conferência das informações extraídas',
        completed: false,
      },
      {
        label: 'Explicação clara do que foi encontrado',
        completed: false,
      },
      {
        label: 'Sinalização do que está faltando',
        completed: false,
      },
      {
        label: 'Sinalização do que precisa de atenção',
        completed: false,
      },
      {
        label: 'Consulta da origem de cada informação',
        completed: false,
      },
      {
        label: 'Revisão do fluxo com o médico',
        completed: false,
      },
      {
        label: 'Melhor adaptação para celulares',
        completed: false,
      },
    ],
  },
  {
    id: 'exames-reais',
    number: 2,
    title: 'Primeira versão com exames reais',
    description:
        'Começar a substituir as simulações por um fluxo real, inicialmente mais simples e limitado.',
    status: 'Próxima etapa',
    badge: 'Vem a seguir',
    items: [
      {
        label: 'Envio real de arquivos',
        completed: false,
      },
      {
        label: 'Leitura inicial de um modelo de Pentacam',
        completed: false,
      },
      {
        label: 'Identificação do nome e da idade do paciente',
        completed: false,
      },
      {
        label: 'Leitura das principais informações do exame',
        completed: false,
      },
      {
        label: 'Primeiro caso real do início ao fim',
        completed: false,
      },
      {
        label: 'Primeiras regras clínicas automatizadas',
        completed: false,
      },
      {
        label: 'Geração de um relatório real',
        completed: false,
      },
      {
        label: 'Histórico dos casos analisados',
        completed: false,
      },
      {
        label: 'Registro da revisão feita pelo médico',
        completed: false,
      },
      {
        label: 'Tratamento das informações que estiverem ausentes',
        completed: false,
      },
    ],
  },
  {
    id: 'conferencia-seguranca',
    number: 3,
    title: 'Conferência e segurança dos dados',
    description:
        'Garantir que cada informação apresentada possa ser conferida de forma rápida e segura.',
    status: 'Mais adiante',
    badge: 'Etapas futuras',
    items: [
      {
        label: 'Mostrar de qual exame veio cada informação',
        completed: false,
      },
      {
        label: 'Mostrar o trecho do exame em que o dado foi encontrado',
        completed: false,
      },
      {
        label: 'Sinalizar a confiança da leitura',
        completed: false,
      },
      {
        label: 'Comparar automaticamente informações relacionadas',
        completed: false,
      },
      {
        label: 'Confirmar se todos os exames pertencem ao mesmo paciente',
        completed: false,
      },
      {
        label: 'Bloquear o relatório quando houver conflito de identidade',
        completed: false,
      },
      {
        label: 'Permitir a conferência dos cálculos',
        completed: false,
      },
      {
        label: 'Registrar qual versão do protocolo foi utilizada',
        completed: false,
      },
      {
        label: 'Manter o histórico das alterações no relatório',
        completed: false,
      },
    ],
  },
  {
    id: 'protocolo-completo',
    number: 4,
    title: 'Aplicação completa do protocolo clínico',
    description:
        'Cobrir todos os caminhos de decisão definidos no protocolo médico.',
    status: 'Mais adiante',
    badge: 'Etapas futuras',
    items: [
      {
        label: 'Pacientes abaixo de 40 anos',
        completed: false,
      },
      {
        label: 'Pacientes entre 40 e 55 anos',
        completed: false,
      },
      {
        label: 'Pacientes acima de 55 anos',
        completed: false,
      },
      {
        label: 'Avaliação para LASIK',
        completed: false,
      },
      {
        label: 'Avaliação para PRK',
        completed: false,
      },
      {
        label: 'Avaliação para lente fácica',
        completed: false,
      },
      {
        label: 'Avaliação para Presbyond e PresbyLASIK',
        completed: false,
      },
      {
        label: 'Avaliação para lentes multifocais e EDOF',
        completed: false,
      },
      {
        label: 'Avaliação do risco de ectasia',
        completed: false,
      },
      {
        label: 'Cálculos de PTA, LER e ceratometria final',
        completed: false,
      },
      {
        label: 'Possibilidade de ajustar parâmetros clínicos',
        completed: false,
      },
      {
        label: 'Apresentação das fontes científicas utilizadas',
        completed: false,
      },
    ],
  },
  {
    id: 'ampliacao',
    number: 5,
    title: 'Ampliação da plataforma',
    description:
        'Adicionar novos exames, recursos e possibilidades de uso para clínicas e profissionais.',
    status: 'Mais adiante',
    badge: 'Etapas futuras',
    items: [
      {
        label: 'Suporte a equipamentos diferentes',
        completed: false,
      },
      {
        label: 'Leitura de biometria óptica',
        completed: false,
      },
      {
        label: 'Leitura de microscopia especular',
        completed: false,
      },
      {
        label: 'Leitura de OCT de retina',
        completed: false,
      },
      {
        label: 'Leitura de retinografia',
        completed: false,
      },
      {
        label: 'Leitura combinada de diferentes tipos de exame',
        completed: false,
      },
      {
        label: 'Painel para ajustes clínicos',
        completed: false,
      },
      {
        label: 'Medição da precisão das leituras',
        completed: false,
      },
      {
        label: 'Acompanhamento de acertos e erros',
        completed: false,
      },
      {
        label: 'Cadastro de profissionais',
        completed: false,
      },
      {
        label: 'Cadastro de clínicas',
        completed: false,
      },
    ],
  },
]

function getProgress(phase: RoadmapPhase) {
  const completed = phase.items.filter((item) => item.completed).length
  const total = phase.items.length

  return {
    completed,
    pending: total - completed,
    percentage: total === 0
        ? 0
        : Math.round((completed / total) * 100),
  }
}

function PhaseBadge({
                      badge,
                    }: {
  badge: RoadmapPhase['badge']
}) {
  return (
      <span
          className={clsx(
              'inline-flex items-center rounded-full px-3 py-1.5 text-xs font-bold',
              badge === 'Estamos aqui' && 'bg-warning-soft text-warning',
              badge === 'Vem a seguir' && 'bg-primary-soft text-primary',
              badge === 'Etapas futuras' && 'bg-surface-muted text-text-secondary',
              badge === 'Concluído' && 'bg-success-soft text-success',
          )}
      >
      {badge}
    </span>
  )
}

function RoadmapProgress({
                           phase,
                           hidePercentage = false,
                         }: {
  phase: RoadmapPhase
  hidePercentage?: boolean
}) {
  const progress = getProgress(phase)
  const current = phase.badge === 'Estamos aqui'

  return (
      <div>
        <div className="mb-3 flex items-center justify-between gap-4 text-sm">
        <span className="font-semibold text-text-secondary">
          {progress.completed} de {phase.items.length} concluídos
        </span>

          {!hidePercentage && (
              <strong
                  className={clsx(
                      'font-display text-base',
                      current ? 'text-warning' : 'text-text-primary',
                  )}
              >
                {progress.percentage}%
              </strong>
          )}
        </div>

        <div
            aria-label={`Progresso de ${phase.title}: ${progress.percentage}%`}
            aria-valuemax={100}
            aria-valuemin={0}
            aria-valuenow={progress.percentage}
            className="h-2.5 overflow-hidden rounded-full bg-border"
            role="progressbar"
        >
          <div
              className={clsx(
                  'h-full rounded-full transition-[width] duration-500',
                  current ? 'bg-warning' : 'bg-primary',
              )}
              style={{ width: `${progress.percentage}%` }}
          />
        </div>
      </div>
  )
}

function JourneyPhaseCard({
                            phase,
                          }: {
  phase: RoadmapPhase
}) {
  const progress = getProgress(phase)
  const current = phase.badge === 'Estamos aqui'

  return (
      <article
          className={clsx(
              'relative flex min-w-0 flex-col rounded-2xl border bg-surface p-5 shadow-sm',
              current
                  ? 'border-warning'
                  : 'border-border',
          )}
      >
        {current && (
            <div className="absolute inset-x-5 top-0 h-1 rounded-b-full bg-warning" />
        )}

        <div className="flex items-center justify-between gap-3">
        <span
            className={clsx(
                'grid h-10 w-10 place-items-center rounded-full font-display text-sm font-bold',
                current
                    ? 'bg-warning text-white'
                    : 'bg-primary-soft text-primary',
            )}
        >
          {phase.number}
        </span>

          <PhaseBadge badge={phase.badge} />
        </div>

        <h3 className="mb-0 mt-5 font-display text-base leading-snug tracking-[-0.02em]">
          {phase.title}
        </h3>

        <p className="mb-0 mt-3 text-sm leading-6 text-text-secondary">
          {phase.description}
        </p>

        <div className="mt-auto pt-6">
          <RoadmapProgress phase={phase} />
        </div>

        <span className="mt-3 text-sm text-text-muted">
        {progress.pending === 0
            ? 'Todos os passos foram concluídos.'
            : `${progress.pending} passos ainda serão realizados.`}
      </span>
      </article>
  )
}

function RoadmapPhaseDetails({
                               phase,
                               expanded,
                               onToggle,
                             }: {
  phase: RoadmapPhase
  expanded: boolean
  onToggle: () => void
}) {
  const current = phase.badge === 'Estamos aqui'
  const progress = getProgress(phase)
  const detailsId = `roadmap-details-${phase.id}`

  return (
      <article
          className={clsx(
              'relative overflow-hidden rounded-2xl border bg-surface shadow-sm',
              current
                  ? 'border-warning'
                  : 'border-border',
          )}
      >
        {current && (
            <div className="absolute inset-y-0 left-0 w-1.5 bg-warning" />
        )}

        <div className="p-6 max-[580px]:p-5">
          <button
              aria-controls={detailsId}
              aria-expanded={expanded}
              className="flex w-full items-start gap-4 border-0 bg-transparent p-0 text-left"
              onClick={onToggle}
              type="button"
          >
          <span
              className={clsx(
                  'grid h-12 w-12 flex-none place-items-center rounded-xl border font-display text-base font-bold',
                  current
                      ? 'border-warning bg-warning-soft text-warning'
                      : 'border-border bg-surface-muted text-text-secondary',
              )}
          >
            {phase.number.toString().padStart(2, '0')}
          </span>

            <span className="min-w-0 flex-1">
            <span className="flex flex-wrap items-center justify-between gap-3">
              <strong className="font-display text-xl tracking-[-0.025em]">
                {phase.title}
              </strong>

              <PhaseBadge badge={phase.badge} />
            </span>

            <span className="mt-2 block max-w-[900px] text-sm leading-6 text-text-secondary">
              {phase.description}
            </span>
          </span>

            <ChevronDown
                aria-hidden="true"
                className={clsx(
                    'mt-2 flex-none text-primary transition-transform duration-300',
                    expanded && 'rotate-180',
                )}
                size={22}
            />
          </button>

          <div className="mt-6">
            <RoadmapProgress phase={phase} />
          </div>

          <div className="mt-4 flex flex-wrap gap-x-8 gap-y-2 text-sm">
          <span className="text-text-secondary">
            Status:{' '}
            <strong
                className={current ? 'text-warning' : 'text-text-primary'}
            >
              {phase.status}
            </strong>
          </span>

            <span className="text-text-secondary">
            Pendentes:{' '}
              <strong className="text-text-primary">
              {progress.pending}
            </strong>
          </span>
          </div>

          <div
              aria-hidden={!expanded}
              className={clsx(
                  'grid transition-[grid-template-rows,opacity] duration-300',
                  expanded
                      ? 'grid-rows-[1fr] opacity-100'
                      : 'grid-rows-[0fr] opacity-0',
              )}
              id={detailsId}
          >
            <div className="overflow-hidden">
              <div className="mt-7 border-t border-border pt-6">
                <h4 className="m-0 font-display text-lg">
                  {current
                      ? 'O que estamos fazendo nesta etapa'
                      : 'O que está previsto para esta etapa'}
                </h4>

                <p className="mb-0 mt-2 text-sm leading-6 text-text-secondary">
                  {current
                      ? 'Estes são os passos que estamos validando antes de iniciar a automação completa.'
                      : 'Estes itens serão desenvolvidos quando o projeto chegar a esta etapa.'}
                </p>

                <ul className="mb-0 mt-5 grid list-none grid-cols-2 gap-3 p-0 max-[820px]:grid-cols-1">
                  {phase.items.map((item) => (
                      <li
                          className={clsx(
                              'flex min-h-14 items-start gap-3 rounded-xl border px-4 py-3.5 text-sm leading-6',
                              item.completed
                                  ? 'border-success bg-success-soft text-text-primary'
                                  : 'border-border bg-surface-muted text-text-secondary',
                          )}
                          key={item.label}
                      >
                        {item.completed ? (
                            <Check
                                aria-hidden="true"
                                className="mt-1 flex-none text-success"
                                size={18}
                                strokeWidth={3}
                            />
                        ) : (
                            <Circle
                                aria-hidden="true"
                                className="mt-1 flex-none text-text-muted"
                                size={18}
                            />
                        )}

                        <span>
                      {item.label}

                          <span className="sr-only">
                        {' '}
                            — {item.completed ? 'concluído' : 'pendente'}
                      </span>
                    </span>
                      </li>
                  ))}
                </ul>
              </div>
            </div>
          </div>
        </div>
      </article>
  )
}

export default function RoadmapPage() {
  const [expandedPhase, setExpandedPhase] = useState<string | null>(
      phases[0].id,
  )

  const currentPhase = phases[0]
  const currentProgress = getProgress(currentPhase)

  return (
      <div className="min-w-0">
        <section className="hero-bg overflow-hidden rounded-2xl border border-primary-border p-7 max-[580px]:p-5">
          <div className="grid grid-cols-[minmax(0,1.3fr)_minmax(280px,.7fr)] items-center gap-10 max-[900px]:grid-cols-1">
            <div>
            <span className="text-xs font-bold tracking-[0.13em] text-primary">
              VISÃO DO PROJETO
            </span>

              <h2 className="mb-0 mt-3 font-display text-[clamp(28px,3vw,40px)] leading-[1.12] tracking-[-0.045em]">
                Roadmap RefratIA
              </h2>

              <p className="mb-0 mt-5 max-w-[760px] text-sm leading-7 text-text-secondary">
                O RefratIA será construído em etapas. Primeiro, vamos validar
                a melhor experiência de uso com médicos. Depois, vamos
                transformar essa experiência em uma solução técnica completa.
              </p>
            </div>

            <blockquote className="m-0 rounded-2xl border border-primary-border bg-surface p-6 shadow-sm">
              <Sparkles
                  aria-hidden="true"
                  className="text-primary"
                  size={26}
              />

              <p className="mb-0 mt-5 font-display text-xl leading-snug tracking-[-0.03em]">
                Primeiro a experiência.
                <br />
                Depois a automação.
              </p>

              <footer className="mt-3 text-xs leading-5 text-text-secondary">
                Validar o uso real antes de aumentar a complexidade.
              </footer>
            </blockquote>
          </div>
        </section>

        <section
            aria-labelledby="roadmap-now"
            className="mt-6 rounded-2xl border border-warning bg-surface p-7 shadow-sm max-[580px]:p-5"
        >
          <div className="grid grid-cols-[minmax(0,1.2fr)_minmax(300px,.8fr)] items-center gap-8 max-[900px]:grid-cols-1">
            <div>
              <div className="flex items-center gap-4">
              <span className="grid h-12 w-12 flex-none place-items-center rounded-xl bg-warning-soft text-warning">
                <Flag size={22} />
              </span>

                <div>
                <span className="text-xs font-bold tracking-[0.12em] text-warning">
                  ONDE ESTAMOS AGORA
                </span>

                  <h2
                      className="mb-0 mt-1 font-display text-xl tracking-[-0.03em]"
                      id="roadmap-now"
                  >
                    Testes de uso com médicos
                  </h2>
                </div>
              </div>

              <p className="mb-0 mt-5 max-w-[760px] text-sm leading-7 text-text-secondary">
                Estamos testando a melhor forma de apresentar exames,
                informações e recomendações médicas antes de automatizar toda
                a parte técnica.
              </p>

              <div className="mt-5">
                <PhaseBadge badge="Estamos aqui" />
              </div>
            </div>

            <div className="rounded-2xl border border-border bg-surface-muted p-5">
            <span className="text-sm font-semibold text-text-secondary">
              Progresso desta etapa
            </span>

              <div className="mt-3 flex items-end justify-between gap-4">
                <strong className="font-display text-2xl tracking-[-0.035em]">
                  {currentProgress.completed} de {currentPhase.items.length}
                </strong>

                <strong className="font-display text-2xl text-warning">
                  {currentProgress.percentage}%
                </strong>
              </div>

              <div className="mt-5">
                <RoadmapProgress
                    hidePercentage
                    phase={currentPhase}
                />
              </div>

              <p className="mb-0 mt-4 text-sm text-text-secondary">
                {currentProgress.pending} passos ainda precisam ser validados.
              </p>
            </div>
          </div>
        </section>

        <section
            aria-labelledby="roadmap-journey"
            className="mt-10"
        >
          <div className="flex items-end justify-between gap-5">
            <div>
            <span className="text-xs font-bold tracking-[0.13em] text-primary">
              ETAPAS DO PROJETO
            </span>

              <h2
                  className="mb-0 mt-2 font-display text-2xl tracking-[-0.035em]"
                  id="roadmap-journey"
              >
                Do primeiro teste ao produto completo
              </h2>
            </div>

            <Route
                aria-hidden="true"
                className="flex-none text-primary"
                size={30}
            />
          </div>

          <p className="mb-0 mt-4 max-w-[820px] text-sm leading-7 text-text-secondary">
            Cada etapa será validada antes da próxima. Assim, o produto evolui
            com clareza, sem implementar complexidade antes de confirmar o que
            realmente faz sentido no uso médico.
          </p>

          <div className="mt-7 grid grid-cols-5 gap-4 max-[1200px]:grid-cols-2 max-[680px]:grid-cols-1">
            {phases.map((phase) => (
                <JourneyPhaseCard
                    key={phase.id}
                    phase={phase}
                />
            ))}
          </div>
        </section>

        <section
            aria-labelledby="roadmap-details"
            className="mt-12"
        >
          <div>
          <span className="text-xs font-bold tracking-[0.13em] text-primary">
            DETALHES
          </span>

            <h2
                className="mb-0 mt-2 font-display text-2xl tracking-[-0.035em]"
                id="roadmap-details"
            >
              O que será feito em cada etapa
            </h2>

            <p className="mb-0 mt-4 max-w-[760px] text-sm leading-7 text-text-secondary">
              Abra uma etapa para ver os passos previstos. A etapa atual já
              começa aberta.
            </p>
          </div>

          <div className="mt-7 grid gap-4">
            {phases.map((phase) => (
                <RoadmapPhaseDetails
                    expanded={expandedPhase === phase.id}
                    key={phase.id}
                    onToggle={() => {
                      setExpandedPhase((current) =>
                          current === phase.id
                              ? null
                              : phase.id,
                      )
                    }}
                    phase={phase}
                />
            ))}
          </div>
        </section>

        <section className="mt-10 grid grid-cols-2 gap-6 max-[900px]:grid-cols-1">
          <article className="rounded-2xl border border-border bg-surface p-7 shadow-sm">
          <span className="grid h-12 w-12 place-items-center rounded-xl bg-primary-soft text-primary">
            <Compass size={23} />
          </span>

            <h2 className="mb-0 mt-5 font-display text-2xl tracking-[-0.03em]">
              Como entender este roadmap
            </h2>

            <p className="mb-0 mt-4 text-sm leading-7 text-text-secondary">
              Este roadmap mostra a evolução esperada do RefratIA. Ele não é
              uma promessa rígida de prazo. Cada etapa será testada antes da
              próxima, para reduzir riscos e evitar trabalho desnecessário.
            </p>
          </article>

          <article className="rounded-2xl border border-primary-border bg-primary-soft p-7 shadow-sm">
          <span className="grid h-12 w-12 place-items-center rounded-xl bg-surface text-primary">
            <ShieldCheck size={23} />
          </span>

            <h2 className="mb-0 mt-5 font-display text-2xl tracking-[-0.03em]">
              Norte clínico
            </h2>

            <p className="mb-0 mt-4 text-sm leading-7 text-text-secondary">
              O RefratIA está sendo construído com base no protocolo clínico
              de cirurgia refrativa. Esse protocolo orienta o produto final.
            </p>

            <ul className="mb-0 mt-6 grid list-none gap-3 p-0">
              {[
                'Os dados não serão inventados',
                'Os dados calculados serão identificados',
                'O que estiver ausente continuará explícito',
                'As decisões poderão ser conferidas',
                'A revisão médica continuará central',
              ].map((principle) => (
                  <li
                      className="flex items-start gap-3 text-base leading-7 text-text-secondary"
                      key={principle}
                  >
                    <Check
                        aria-hidden="true"
                        className="mt-1 flex-none text-primary"
                        size={18}
                        strokeWidth={3}
                    />

                    {principle}
                  </li>
              ))}
            </ul>
          </article>
        </section>
      </div>
  )
}