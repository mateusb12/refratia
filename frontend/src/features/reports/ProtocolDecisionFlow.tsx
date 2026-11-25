import clsx from 'clsx'
import { Check } from 'lucide-react'
import { Eyebrow, StatusBadge } from '../../shared/ui'
import { CONDUTA, ENDOTHELIAL_MIN_CELLS_MM2, TORIC_MIN_ASTIGMATISM_D, coma4mm, evaluateProtocol, type Tone } from '../protocol/protocolDecision'
import { FLOW_B_MIN_AGE, FLOW_C_MIN_AGE, MIN_PROTOCOL_AGE } from '../protocol/clinicalFlow'
import { eyeLabel } from '../../shared/contracts/patient-analysis'
import { formatNumber, type ReportData } from './reportModel'

function FlowConnector({
  variant,
  className,
}: {
  variant: 'straight' | 'to-active' | 'to-center' | 'from-left' | 'from-right'
  className?: string
}) {
  const paths = {
    straight: 'M50 0 V48',
    'to-active': 'M50 0 V16 C50 21 52 24 56 24 H77 C81 24 83 27 83 32 V48',
    'to-center': 'M83 0 V16 C83 21 81 24 77 24 H56 C52 24 50 27 50 32 V48',
    'from-left': 'M25 0 V16 C25 21 27 24 31 24 H44 C48 24 50 27 50 32 V48',

    'from-right': 'M75 0 V16 C75 21 73 24 69 24 H56 C52 24 50 27 50 32 V48',
  }
  const arrowheads = {
    straight: 'M49.25 39 L50 48 L50.75 39 L50 41 Z',
    'to-active': 'M82.25 39 L83 48 L83.75 39 L83 41 Z',
    'to-center': 'M49.25 39 L50 48 L50.75 39 L50 41 Z',
    'from-left': 'M49.25 39 L50 48 L50.75 39 L50 41 Z',
    'from-right': 'M49.25 39 L50 48 L50.75 39 L50 41 Z',
  }

  const renderSvg = (path: string, arrowhead: string, svgClassName?: string) => (
    <svg className={clsx('h-full w-full', svgClassName)} preserveAspectRatio="none" viewBox="0 0 100 48">
      <path d={path} fill="none" shapeRendering="geometricPrecision" stroke="currentColor" strokeLinecap="round" strokeWidth="1.7" vectorEffect="non-scaling-stroke" />
      <path d={arrowhead} fill="currentColor" />
    </svg>
  )

  return (
    <div aria-hidden="true" className={clsx('h-14 w-full text-primary', className)}>
      {variant === 'to-active' ? (
        <>
          {renderSvg(paths[variant], arrowheads[variant], 'max-[820px]:hidden')}
          {renderSvg(paths.straight, arrowheads.straight, 'hidden max-[820px]:block')}
        </>
      ) : renderSvg(paths[variant], arrowheads[variant])}
    </div>
  )
}

const toneCard: Record<Tone, string> = {
  green: 'border-success/40 bg-success-soft',
  yellow: 'border-warning/50 bg-warning-soft',
  red: 'border-danger/50 bg-danger-soft',
  gray: 'border-border bg-surface-muted',
}

export default function ProtocolDecisionFlow({ data }: { data: ReportData }) {
  const decision = evaluateProtocol(data)
  const eyes = (['OD', 'OS'] as const).map((eye) => {
    const pentacam = data.exams.pentacam_corneal_tomography.eyes[eye]
    const coma = Number.parseFloat(pentacam.corneal_rings.zernike['5mm'].z31_coma)
    const coma4 = coma4mm(pentacam)
    const evaluated = decision.eyes[eye]

    return {
      eye,
      evaluated,
      astigmatism: evaluated.astigmatism.value ?? Number.NaN,
      astigmatismKnown: evaluated.astigmatism.status === 'aprovado',
      endothelialDensity: evaluated.endothelium.value ?? Number.NaN,
      endotheliumPending: evaluated.endothelium.status === 'pendente' || evaluated.endothelium.status === 'nao_avaliado',
      coma,
      coma4,
      badD: pentacam.belin_ambrosio.d,
      artMax: pentacam.belin_ambrosio.art_max,
      tkc: pentacam.topometric_indices_8mm.tkc,
      z40: pentacam.cataract_preop.total_corneal_z40_6mm_um,
    }
  })

  const { age, flow } = decision
  const flowNotice = flow === 'out_of_scope'
    ? 'Fora do escopo do protocolo (menor de 18 anos). Nenhum fluxo se aplica.'
    : decision.state === 'keratoconus'
      ? 'Regra 2.1: TKC classificado — cascata encerrada para o paciente inteiro, em qualquer fluxo; encaminhamento a especialista em córnea. Nenhuma cirurgia é indicada automaticamente.'
      : decision.state === 'screening_incomplete' && (flow === 'A' || flow === 'B')
        ? `TKC não lido em pelo menos um olho: o rastreio de ceratocone (regra 2.1) não foi concluído, e o protocolo não presume rastreio normal. A cascata do Fluxo ${flow} para aqui; nenhuma cirurgia é indicada até conferir a tela do Pentacam.`
        : 'Idade não determinada: revise a identidade e a data do exame Pentacam. Nenhum fluxo foi escolhido.'
  const ageText = age === null ? 'Paciente com idade não informada' : `Paciente com ${age} anos`
  const keratoconus = decision.tkc
  const tkcStatus = keratoconus.status

  const hasEndothelialAlert = eyes.some(({ evaluated }) => evaluated.endothelium.status === 'condiciona_conduta')
  const hasEndothelialPending = eyes.some(({ endotheliumPending }) => endotheliumPending)
  const hasToricNeed = eyes.some(({ evaluated }) => evaluated.toric === true)
  const hasToricUnknown = eyes.some(({ evaluated }) => evaluated.toric === null)
  const sent = (key: string) => Boolean((data.exams as Record<string, { source?: unknown[] } | undefined>)[key]?.source?.length)
  const ocrSent = sent('oct_retina')
  const retinographySent = sent('fundus_retinography')

  return (
    <section className="mt-5 rounded-2xl border border-border bg-surface p-6 shadow-sm max-[580px]:p-4">
      <Eyebrow>ETAPA 4 · COMO O PROTOCOLO CHEGOU À INDICAÇÃO</Eyebrow>
      <div className="mt-1 flex items-start justify-between gap-4 max-[820px]:flex-col">
        <div>
          <h2 className="m-0 font-display text-2xl tracking-[-0.03em]">Acompanhe a decisão, passo a passo</h2>
          <p className="mb-0 mt-2 max-w-[760px] text-sm leading-relaxed text-text-secondary">
            Cada etapa mostra a pergunta do protocolo, o dado usado e o efeito na recomendação.
          </p>
        </div>
      </div>

      <div aria-label="Fluxograma da decisão do protocolo" className="mt-6">
        <div className="mx-auto max-w-[760px] rounded-xl border border-primary bg-primary-soft p-4">
          <span className="text-xs font-bold tracking-[0.12em] text-primary">INÍCIO</span>
          <strong className="mt-1 block text-base">
            {age === null ? 'Idade: não informada' : `Idade: ${age} anos`} → {flow === 'out_of_scope' ? 'fora do escopo do protocolo' : flow ? `o protocolo escolhe o Fluxo ${flow}` : 'nenhum fluxo escolhido'}
          </strong>
          {flow === 'C' && <span className="mt-1 block text-xs text-text-secondary">Regra: a partir de {FLOW_C_MIN_AGE} anos, LASIK e PRK não entram na cascata.</span>}
        </div>

        <FlowConnector variant="to-active" />

        <div className="grid grid-cols-3 gap-3 max-[820px]:grid-cols-1">
          {[
            { id: 'A', title: 'Córnea ou lente fácica', minAge: MIN_PROTOCOL_AGE, maxAge: FLOW_B_MIN_AGE - 1 },
            { id: 'B', title: 'Presbiopia', minAge: FLOW_B_MIN_AGE, maxAge: FLOW_C_MIN_AGE - 1 },
          ].map((card) => (
            <article className="min-h-[190px] rounded-xl border border-border bg-surface-muted p-4 text-text-muted" key={card.id} title={`Aplicável a pacientes de ${card.minAge} a ${card.maxAge} anos.`}>
              <span className="text-xs font-bold tracking-[0.12em]">FLUXO {card.id}</span>
              <h3 className="mb-0 mt-1 text-base font-bold">{card.title}</h3>

              <div className="mt-3 grid gap-2">
                <div className="rounded-lg border border-border bg-surface/60 px-3 py-2">
                  <span className="block text-[10px] font-bold uppercase tracking-[0.10em] text-text-muted">
                    Regra do fluxo
                  </span>
                  <strong className="mt-1 block text-sm text-text-secondary">
                    Idade entre {card.minAge} e {card.maxAge} anos
                  </strong>
                </div>

                {flow === card.id ? (
                  <div className="rounded-lg bg-success-soft px-3 py-2">
                    <span className="block text-[10px] font-bold uppercase tracking-[0.10em] text-success">ROTA ATIVA · FLUXO {card.id}</span>
                    <strong className="mt-1 block text-sm text-text-primary">{ageText}</strong>
                  </div>
                ) : (
                  <div className="rounded-lg bg-warning-soft/40 px-3 py-2">
                    <span className="block text-[10px] font-bold uppercase tracking-[0.10em] text-warning">Não escolhido</span>
                    <strong className="mt-1 block text-sm text-text-primary">{ageText}</strong>
                  </div>
                )}
              </div>
            </article>
          ))}

          <article className={clsx('min-h-[134px] rounded-xl border p-4', flow === 'C' ? 'border-success/40 bg-success-soft shadow-sm' : 'border-border bg-surface-muted text-text-muted')}>
            <div className="flex items-start justify-between gap-3">
              <div>
                <span className={clsx('text-xs font-bold tracking-[0.12em]', flow === 'C' && 'text-success')}>{flow === 'C' ? 'ROTA ATIVA · FLUXO C' : 'FLUXO C'}</span>
                <h3 className="mb-0 mt-1 text-base font-bold">LIO multifocal ou EDOF</h3>
              </div>
              {flow === 'C' && <StatusBadge tone="success"><Check size={12} /> Selecionado</StatusBadge>}
            </div>
            {flow === 'C'
              ? <p className="mb-0 mt-2 text-xs leading-relaxed text-text-secondary">A idade define a categoria da indicação. A escolha entre multifocal e EDOF fica com o cirurgião.</p>
              : <p className="mb-0 mt-2 text-xs font-bold text-warning">Não escolhido · {ageText}</p>}
          </article>
        </div>

        {flow !== 'C' && decision.state === 'evaluated' && (
          <div className="mx-auto mt-6 max-w-[760px] rounded-xl border border-primary-border bg-surface p-5" role="status">
            <span className="text-xs font-bold tracking-[0.12em] text-primary">FLUXO {flow} · CASCATA AVALIADA POR OLHO</span>
            <p className="mb-0 mt-2 text-sm leading-relaxed text-text-primary">
              {flow === 'A'
                ? 'Regra 2.1 não dispara. Etapas do Fluxo A (seção 5): faixa dióptrica → LASIK e PRK (LER, PTA, K_final) → alerta de qualidade visual noturna → lente fácica → anel estromal + PRK. Cada olho abaixo mostra o dado usado em cada etapa.'
                : 'Regra 2.1 não dispara. Fluxo B (seção 6): córnea apta pelos critérios de LER, PTA e K_final (com +2,00 D do alvo de perto) → faixa refracional de Presbyond e Custom-Q/PresbyLASIK; córnea não apta → ambulatório do cirurgião. Cada olho abaixo mostra o dado usado em cada etapa.'}
            </p>
            <p className="mb-0 mt-2 text-xs leading-relaxed text-text-secondary">
              Esta tela ainda não emite as observações informativas de vias de laser duplas, registro de coma, PNS, Z40 e evidência do anel. Elas não alteram a conduta.
            </p>
          </div>
        )}
        {flow !== 'C' && decision.state !== 'evaluated' && (
          <div className="mx-auto mt-6 max-w-[760px] rounded-xl border border-warning/50 bg-warning-soft p-5" role="status">
            <span className="text-xs font-bold tracking-[0.12em] text-warning">{decision.state === 'keratoconus' ? 'REGRA 2.1' : 'SEM RECOMENDAÇÃO AUTOMÁTICA'}</span>
            <p className="mb-0 mt-2 text-sm leading-relaxed text-text-primary">{flowNotice}</p>
          </div>
        )}
        {flow === 'C' && (
<>
        <FlowConnector variant="to-center" className="max-[820px]:hidden" />
        <FlowConnector variant="straight" className="hidden max-[820px]:block" />

        <article className="mx-auto max-w-[760px] rounded-xl border border-primary-border bg-surface p-5 shadow-sm">
          <div className="flex items-start justify-between gap-3">
            <div>
              <span className="text-xs font-bold tracking-[0.12em] text-primary">DECISÃO AVALIADA</span>
              <h3 className="mb-0 mt-1 text-[1.05rem] font-bold">Há ceratocone confirmado?</h3>
            </div>
            {tkcStatus === 'classified'
              ? <StatusBadge tone="warning">Sim</StatusBadge>
              : tkcStatus === 'not_classified'
                ? <StatusBadge tone="success"><Check size={12} /> Não</StatusBadge>
                : <StatusBadge tone="warning">Indeterminado — TKC não lido</StatusBadge>}
          </div>

          <div className="mt-4 grid grid-cols-2 gap-4 max-[640px]:grid-cols-1">
            {eyes.map(({ eye, badD, artMax, tkc }) => {
              const eyeStatus = keratoconus.eyes[eye]
              const suspicious = eyeStatus === 'classified'
              return (
                <div className="rounded-xl border border-border bg-surface-muted p-4" key={eye}>
                  <div className="flex items-start justify-between gap-3">
                    <strong className="text-base">{eyeLabel(eye)}</strong>
                    <span
                      className={clsx(
                        'inline-flex rounded-full px-2.5 py-1 text-xs font-bold',
                        suspicious ? 'bg-warning-soft/60 text-warning' : 'bg-surface text-text-secondary',
                      )}
                    >
                      {suspicious ? 'TKC classificado' : eyeStatus === 'not_classified' ? 'Sem TKC classificado' : 'TKC não lido'}
                    </span>
                  </div>

                  <div className="mt-4 grid gap-3 text-sm">
                    <div className="grid grid-cols-[88px_1fr] items-center gap-3">
                      <span className="text-text-muted">BAD-D</span>
                      <strong className="text-text-primary">{formatNumber(badD)}</strong>
                    </div>

                    <div className="grid grid-cols-[88px_1fr] items-center gap-3">
                      <span className="text-text-muted">ARTmax</span>
                      <strong className="text-text-primary">{formatNumber(artMax)} µm</strong>
                    </div>

                    <div className="grid grid-cols-[88px_1fr] items-center gap-3">
                      <span className="text-text-muted">TKC</span>
                      <strong className="text-text-primary">{keratoconus.eyes[eye] === 'classified' ? tkc : keratoconus.eyes[eye] === 'not_classified' ? 'não classificado como ceratocone' : 'não avaliado — exame pendente'}</strong>
                    </div>
                  </div>

                  <div
                    className={clsx(
                      'mt-4 rounded-lg px-3 py-2 text-sm font-semibold',
                      suspicious ? 'bg-warning-soft/40 text-warning' : 'bg-surface text-text-secondary',
                    )}
                  >
                    {suspicious ? 'Ceratocone classificado (TKC)' : eyeStatus === 'not_classified' ? 'Não classificado como ceratocone' : 'Indeterminado — TKC não lido'}
                  </div>
                </div>
              )
            })}
          </div>

          <p className="mb-0 mt-4 text-sm leading-relaxed text-text-secondary">
            {tkcStatus === 'classified'
              ? 'TKC classificado em pelo menos um olho: a regra 2.1 fecha a cascata para o paciente.'
              : tkcStatus === 'not_classified'
                ? 'Nenhum TKC classificado: a regra global não exclui a indicação.'
                : 'TKC não lido ou ilegível: ceratocone indeterminado (não equivale a "Não"). O rastreio da regra 2.1 não foi concluído; confira a tela do Pentacam.'}
          </p>

          <div className="mt-4 grid grid-cols-2 gap-4 max-[640px]:grid-cols-1">
            <div
              className="min-h-[120px] rounded-xl border border-border bg-surface-muted p-4"
              title="Regra 2.1: TKC classificado encerra a cascata."
            >
              <strong className="block text-sm text-text-secondary">Sim</strong>
              <p className="mb-0 mt-2 text-sm leading-relaxed text-text-secondary">
                Cascata encerrada (regra 2.1); encaminhar a especialista em córnea.
              </p>
            </div>

            <div className={clsx('min-h-[120px] rounded-xl border p-4', tkcStatus === 'not_classified' ? 'border-success/40 bg-success-soft' : 'border-border bg-surface-muted')}>
              <span className={clsx('flex items-center gap-1 text-sm font-bold', tkcStatus === 'not_classified' ? 'text-success' : 'text-text-secondary')}>
                {tkcStatus === 'not_classified' ? <><Check size={14} /> Não · rota escolhida</> : tkcStatus === 'unknown' ? 'Indeterminado — TKC não lido' : 'Não'}
              </span>
              <p className="mb-0 mt-2 text-base font-semibold text-text-primary">
                Seguir para a escolha da lente
              </p>
            </div>
          </div>
        </article>

        {tkcStatus === 'not_classified' && (
<>
        <FlowConnector
          variant="from-right"
          className="mx-auto max-w-[760px] max-[580px]:hidden"
        />
        <FlowConnector
          variant="straight"
          className="mx-auto hidden max-w-[760px] max-[580px]:block"
        />

        <div className="mx-auto max-w-[760px] rounded-xl border border-primary-border bg-surface p-4">
          <span className="text-xs font-bold tracking-[0.12em] text-primary">DECISÃO AVALIADA</span>
          <h3 className="mb-0 mt-1 text-base font-bold">Algum olho está abaixo do ponto de corte endotelial?</h3>
          <p className="mb-0 mt-1 text-xs text-text-muted">
            Regra: densidade celular &lt; {ENDOTHELIAL_MIN_CELLS_MM2.toLocaleString('pt-BR')} células/mm² → conduta do olho condicionada (“a considerar após avaliação endotelial”). Microscopia ausente, ilegível ou fora da janela de 4 meses → exame pendente.
          </p>

          <div className="mt-4 grid grid-cols-2 gap-3 max-[580px]:grid-cols-1">
            {eyes.map(({ eye, endothelialDensity, endotheliumPending, evaluated }) => {
              if (endotheliumPending) {
                return (
                  <div className="rounded-xl border border-warning/50 bg-warning-soft/40 p-4" data-testid={`endothelium-${eye}`} key={eye}>
                    <div className="flex items-start justify-between gap-3">
                      <span className="text-xs font-bold tracking-[0.12em] text-text-secondary">{eyeLabel(eye)}</span>
                      <StatusBadge tone="warning">Exame pendente</StatusBadge>
                    </div>
                    <p className="mb-0 mt-3 text-sm font-semibold text-text-primary">{evaluated.endothelium.text}</p>
                  </div>
                )
              }
              const belowCutoff = evaluated.endothelium.status === 'condiciona_conduta'
              const scaleMax = Math.max(
                ENDOTHELIAL_MIN_CELLS_MM2 * 1.5,
                ...eyes.map(({ endothelialDensity: density }) => density).filter(Number.isFinite),
              )
              const cutoffWidth = (ENDOTHELIAL_MIN_CELLS_MM2 / scaleMax) * 100
              const patientWidth = (endothelialDensity / scaleMax) * 100
              const distanceFromCutoff = Math.abs(endothelialDensity - ENDOTHELIAL_MIN_CELLS_MM2)

              return (
                <div
                  className={clsx(
                    'rounded-xl border p-4',
                    belowCutoff
                      ? 'border-warning/40 bg-warning-soft/30'
                      : 'border-success/40 bg-success-soft/30',
                  )}
                  data-testid={`endothelium-${eye}`}
                  key={eye}
                >
                  <div className="flex items-start justify-between gap-3">
                    <div>
                      <span className="text-xs font-bold tracking-[0.12em] text-text-secondary">
                        {eyeLabel(eye)}
                      </span>
                      <strong className="mt-1 block font-display text-xl">
                        {formatNumber(endothelialDensity)} células/mm²
                      </strong>
                    </div>

                    <StatusBadge tone={belowCutoff ? 'warning' : 'success'}>
                      {belowCutoff ? 'Abaixo do corte' : 'Acima do corte'}
                    </StatusBadge>
                  </div>

                  <div className="mt-5 grid gap-4">
                    <div>
                      <div className="flex items-baseline justify-between gap-3">
                        <span className="text-xs font-semibold text-text-secondary">
                          Ponto de corte
                        </span>
                        <strong className="text-xs text-text-primary">
                          {ENDOTHELIAL_MIN_CELLS_MM2.toLocaleString('pt-BR')}
                        </strong>
                      </div>

                      <div className="relative mt-2 h-3 overflow-hidden rounded-full bg-surface-muted">
                        <div
                          className="h-full rounded-full bg-primary"
                          style={{ width: `${cutoffWidth}%` }}
                        />
                        <span
                          aria-hidden="true"
                          className="absolute inset-y-0 w-0.5 bg-text-primary/70"
                          style={{ left: `calc(${cutoffWidth}% - 1px)` }}
                        />
                      </div>
                    </div>

                    <div>
                      <div className="flex items-baseline justify-between gap-3">
                        <span className="text-xs font-semibold text-text-secondary">
                          Paciente
                        </span>
                        <strong className={clsx(
                          'text-xs',
                          belowCutoff ? 'text-warning' : 'text-success',
                        )}>
                          {formatNumber(endothelialDensity)}
                        </strong>
                      </div>

                      <div className="relative mt-2 h-3 overflow-hidden rounded-full bg-surface-muted">
                        <div
                          className={clsx(
                            'h-full rounded-full',
                            belowCutoff ? 'bg-warning' : 'bg-primary',
                          )}
                          style={{ width: `${belowCutoff ? patientWidth : Math.min(patientWidth, cutoffWidth)}%` }}
                        />
                        {!belowCutoff && patientWidth > cutoffWidth && (
                          <div
                            aria-hidden="true"
                            className="absolute inset-y-0 rounded-r-full bg-warning"
                            style={{
                              left: `${cutoffWidth}%`,
                              width: `${patientWidth - cutoffWidth}%`,
                            }}
                          />
                        )}
                        <span
                          aria-hidden="true"
                          className="absolute inset-y-0 w-0.5 bg-text-primary/70"
                          style={{ left: `calc(${cutoffWidth}% - 1px)` }}
                        />
                      </div>
                    </div>
                  </div>

                  <div className={clsx(
                    'mt-4 rounded-lg px-3 py-2 text-xs font-semibold',
                    belowCutoff
                      ? 'bg-warning-soft text-warning'
                      : 'bg-success-soft text-success',
                  )}>
                    {belowCutoff
                      ? `${formatNumber(distanceFromCutoff)} células/mm² abaixo do corte`
                      : `${formatNumber(distanceFromCutoff)} células/mm² acima do corte`}
                  </div>
                </div>
              )
            })}
          </div>

          <div className="mt-4 grid grid-cols-2 gap-3 max-[580px]:grid-cols-1">
            <div
              className={clsx(
                'min-h-[112px] rounded-lg border p-3 text-xs',
                hasEndothelialAlert
                  ? 'border-warning/50 bg-warning-soft'
                  : 'border-border bg-surface-muted text-text-muted',
              )}
            >
              <span className={clsx(
                'flex items-center gap-1 font-bold',
                hasEndothelialAlert ? 'text-warning' : 'text-text-secondary',
              )}>
                {hasEndothelialAlert && <Check size={13} />}
                Sim
                {hasEndothelialAlert && ' · rota escolhida'}
              </span>

              <strong className="mt-2 block">
                Pelo menos um olho abaixo de {ENDOTHELIAL_MIN_CELLS_MM2.toLocaleString('pt-BR')} células/mm²
              </strong>

              <span className="mt-1 block">
                Conduta condicionada nesse olho (“— a considerar após avaliação endotelial”).
              </span>
            </div>

            <div
              className={clsx(
                'min-h-[112px] rounded-lg border p-3 text-xs',
                !hasEndothelialAlert && !hasEndothelialPending
                  ? 'border-success/50 bg-success-soft'
                  : 'border-border bg-surface-muted text-text-muted',
              )}
            >
              <span className={clsx(
                'flex items-center gap-1 font-bold',
                !hasEndothelialAlert && !hasEndothelialPending ? 'text-success' : 'text-text-secondary',
              )}>
                {!hasEndothelialAlert && !hasEndothelialPending && <Check size={13} />}
                Não
                {!hasEndothelialAlert && !hasEndothelialPending && ' · rota escolhida'}
              </span>

              <strong className="mt-2 block">
                Nenhum olho abaixo de {ENDOTHELIAL_MIN_CELLS_MM2.toLocaleString('pt-BR')} células/mm²
              </strong>

              <span className="mt-1 block">
                Seguir para a próxima decisão do protocolo.
              </span>
            </div>
          </div>

          <p className="mb-0 mt-3 text-[11px] leading-relaxed text-text-muted">
            As barras dos dois olhos usam a mesma escala: quanto maior a barra do paciente em relação ao ponto de corte, maior a margem acima do limite.
          </p>
        </div>

        <FlowConnector
          variant={hasEndothelialAlert ? 'from-left' : 'from-right'}
          className="mx-auto max-w-[760px] max-[580px]:hidden"
        />
        <FlowConnector
          variant="straight"
          className="mx-auto hidden max-w-[760px] max-[580px]:block"
        />

        <article className="mx-auto max-w-[760px] rounded-xl border border-primary-border bg-surface p-4 shadow-sm">
          <span className="text-xs font-bold tracking-[0.12em] text-primary">DECISÃO AVALIADA</span>
          <h3 className="mb-0 mt-1 text-base font-bold">O astigmatismo pede lente tórica?</h3>
          <p className="mb-0 mt-1 text-xs text-text-muted">
            Regra: astigmatismo da biometria óptica ≥ {formatNumber(TORIC_MIN_ASTIGMATISM_D)} D → versão tórica. Sem biometria, o valor não é presumido (o Pentacam nunca substitui): a escolha tórica/não tórica fica pendente.
          </p>

          <div className="mt-4 grid grid-cols-2 gap-3 max-[580px]:grid-cols-1">
            {eyes.map(({ eye, astigmatism, astigmatismKnown, evaluated }) => {
              if (!astigmatismKnown) {
                return (
                  <div className="rounded-xl border border-warning/50 bg-warning-soft/40 p-4" data-testid={`astigmatism-${eye}`} key={eye}>
                    <div className="flex items-start justify-between gap-3">
                      <span className="text-xs font-bold tracking-[0.12em] text-text-secondary">{eyeLabel(eye)}</span>
                      <StatusBadge tone="warning">{evaluated.astigmatism.status === 'pendente' ? 'Exame pendente' : 'Dado ausente'}</StatusBadge>
                    </div>
                    <p className="mb-0 mt-3 text-sm font-semibold text-text-primary">{evaluated.astigmatism.text}</p>
                  </div>
                )
              }
              const isToric = evaluated.toric === true

              const toricScaleMax = Math.max(
                TORIC_MIN_ASTIGMATISM_D * 1.5,
                ...eyes.map(({ astigmatism: value }) => value).filter(Number.isFinite),
              )

              const toricCutoffWidth = (TORIC_MIN_ASTIGMATISM_D / toricScaleMax) * 100
              const toricPatientWidth = (astigmatism / toricScaleMax) * 100
              const toricMargin = Math.abs(astigmatism - TORIC_MIN_ASTIGMATISM_D)

              return (
                <div
                  className={clsx(
                    'rounded-xl border p-4',
                    isToric
                      ? 'border-toric-border bg-toric-soft/40'
                      : 'border-border bg-surface-muted',
                  )}
                  data-testid={`astigmatism-${eye}`}
                  key={eye}
                >
                  <div className="flex items-start justify-between gap-3">
                    <div>
                      <span className="text-xs font-bold tracking-[0.12em] text-text-secondary">
                        {eyeLabel(eye)}
                      </span>

                      <strong className="mt-1 block font-display text-xl">
                        {formatNumber(astigmatism)} D
                      </strong>
                    </div>

                    <span
                      className={clsx(
                        'inline-flex rounded-full px-2.5 py-1.5 text-xs font-bold',
                        isToric
                          ? 'bg-toric-soft text-toric'
                          : 'bg-surface text-text-secondary',
                      )}
                    >
                      {isToric ? 'Acima do corte' : 'Abaixo do corte'}
                    </span>
                  </div>

                  <div className="mt-5 grid gap-4">
                    <div>
                      <div className="flex items-baseline justify-between gap-3">
                        <span className="text-xs font-semibold text-text-secondary">
                          Ponto de corte
                        </span>
                        <strong className="text-xs text-text-primary">
                          {formatNumber(TORIC_MIN_ASTIGMATISM_D)} D
                        </strong>
                      </div>

                      <div className="relative mt-2 h-3 overflow-hidden rounded-full bg-surface-muted">
                        <div
                          className="h-full rounded-full bg-primary"
                          style={{ width: `${toricCutoffWidth}%` }}
                        />
                        <span
                          aria-hidden="true"
                          className="absolute inset-y-0 w-0.5 bg-text-primary/70"
                          style={{
                            left: `calc(${toricCutoffWidth}% - 1px)`,
                          }}
                        />
                      </div>
                    </div>

                    <div>
                      <div className="flex items-baseline justify-between gap-3">
                        <span className="text-xs font-semibold text-text-secondary">
                          Paciente
                        </span>
                        <strong
                          className={clsx(
                            'text-xs',
                            isToric ? 'text-toric' : 'text-text-secondary',
                          )}
                        >
                          {formatNumber(astigmatism)} D
                        </strong>
                      </div>

                      <div className="relative mt-2 h-3 overflow-hidden rounded-full bg-surface-muted">
                        <div
                          className="h-full rounded-full bg-primary"
                          style={{
                            width: `${isToric ? toricCutoffWidth : toricPatientWidth}%`,
                          }}
                        />

                        {isToric && (
                          <div
                            aria-hidden="true"
                            className="absolute inset-y-0 rounded-r-full bg-toric"
                            style={{
                              left: `${toricCutoffWidth}%`,
                              width: `${toricPatientWidth - toricCutoffWidth}%`,
                            }}
                          />
                        )}

                        <span
                          aria-hidden="true"
                          className="absolute inset-y-0 w-0.5 bg-text-primary/70"
                          style={{
                            left: `calc(${toricCutoffWidth}% - 1px)`,
                          }}
                        />
                      </div>
                    </div>
                  </div>

                  <div
                    className={clsx(
                      'mt-4 rounded-lg px-3 py-2 text-xs font-semibold',
                      isToric
                        ? 'bg-toric-soft text-toric'
                        : 'bg-surface text-text-secondary',
                    )}
                  >
                    {isToric
                      ? `+${formatNumber(toricMargin)} D acima do corte · LIO tórica`
                      : `${formatNumber(toricMargin)} D abaixo do corte · LIO não tórica`}
                  </div>
                </div>
              )
            })}
          </div>

          <div className="mt-4 grid grid-cols-2 gap-3 max-[580px]:grid-cols-1">
            <div
              className={clsx(
                'min-h-[112px] rounded-lg border p-3 text-xs',
                hasToricNeed
                  ? 'border-toric-border bg-toric-soft/50'
                  : 'border-border bg-surface-muted text-text-muted',
              )}
            >
              <span
                className={clsx(
                  'flex items-center gap-1 font-bold',
                  hasToricNeed ? 'text-toric' : 'text-text-secondary',
                )}
              >
                {hasToricNeed && <Check size={13} />}
                Sim
                {hasToricNeed && ' · rota escolhida'}
              </span>

              <strong className="mt-2 block">
                Pelo menos um olho com astigmatismo ≥ {formatNumber(TORIC_MIN_ASTIGMATISM_D)} D
              </strong>

              <span className="mt-1 block">
                Usar versão tórica nos olhos que atingiram o corte.
              </span>
            </div>

            <div
              className={clsx(
                'min-h-[112px] rounded-lg border p-3 text-xs',
                !hasToricNeed && !hasToricUnknown
                  ? 'border-success/50 bg-success-soft'
                  : 'border-border bg-surface-muted text-text-muted',
              )}
            >
              <span
                className={clsx(
                  'flex items-center gap-1 font-bold',
                  !hasToricNeed && !hasToricUnknown ? 'text-success' : 'text-text-secondary',
                )}
              >
                {!hasToricNeed && !hasToricUnknown && <Check size={13} />}
                Não
                {!hasToricNeed && !hasToricUnknown && ' · rota escolhida'}
              </span>

              <strong className="mt-2 block">
                Nenhum olho atingiu {formatNumber(TORIC_MIN_ASTIGMATISM_D)} D (com biometria disponível nos dois olhos)
              </strong>

              <span className="mt-1 block">
                LIO não tórica pelo critério de astigmatismo.
              </span>
            </div>
          </div>

          <p className="mb-0 mt-3 text-[11px] leading-relaxed text-text-muted">
            As barras dos dois olhos usam a mesma escala. A faixa roxa mostra quanto o astigmatismo ultrapassou o ponto de corte para toricidade.
          </p>
        </article>

        <FlowConnector
          variant={hasToricNeed ? 'from-left' : 'from-right'}
          className="mx-auto max-w-[760px] max-[580px]:hidden"
        />
        <FlowConnector
          variant="straight"
          className="mx-auto hidden max-w-[760px] max-[580px]:block"
        />

        <details className="mx-auto max-w-[760px] rounded-xl border border-border bg-surface-muted p-4">
          <summary className="cursor-pointer list-none text-sm font-bold marker:hidden">Checagens que não alteraram a rota</summary>
          <div className="mt-3 grid grid-cols-2 gap-3 max-[580px]:grid-cols-1">
            {eyes.map(({ eye, coma, coma4, z40 }) => (
              <ul className="m-0 grid list-none gap-2 rounded-lg border border-border bg-surface p-3 text-xs text-text-secondary" key={eye}>
                <li className="font-bold text-text-primary">{eye}</li>
                <li data-testid={`coma4-${eye}`}>{coma4
                  ? <>Coma (Z31, zona de 4,0 mm, coluna {coma4.column}) {formatNumber(coma4.magnitude, { minimumFractionDigits: 3, maximumFractionDigits: 3 })} µm, magnitude de Coma 0° = {formatNumber(coma4.c0, { minimumFractionDigits: 3, signDisplay: 'exceptZero' })} µm e Coma 90° = {formatNumber(coma4.c90, { minimumFractionDigits: 3, signDisplay: 'exceptZero' })} µm: registro informativo, sem limiar (item 78)</>
                  : 'Coma (Z31, zona de 4,0 mm, coluna WFA Córnea): dado ausente'}</li>
                <li>Coma (Z31, zona de 5 mm) {formatNumber(coma, { minimumFractionDigits: 3 })} µm: leitura da tela Anéis Corneanos, não usada no registro de coma do protocolo</li>
                <li>Z40 {formatNumber(z40, { minimumFractionDigits: 3 })} µm: informativo</li>
              </ul>
            ))}
          </div>
          <p className="mb-0 mt-3 text-xs text-text-muted">{ocrSent ? 'OCT de retina enviado: achados não avaliados automaticamente nesta tela. Informativo — não altera a indicação.' : 'OCT de retina não disponível. A ausência deste dado não implica em qualquer mudança na conduta cirúrgica indicada.'} {retinographySent ? 'Retinografia enviada: achados não lidos automaticamente nesta tela.' : 'Retinografia não enviada: saúde macular não avaliada (não bloqueia a conduta).'}</p>
        </details>
</>
        )}
        {tkcStatus === 'unknown' && (
          <div className="mx-auto mt-6 max-w-[760px] rounded-xl border border-warning/50 bg-warning-soft p-5" role="status">
            <span className="text-xs font-bold tracking-[0.12em] text-warning">PENDENTE</span>
            <p className="mb-0 mt-2 text-sm leading-relaxed text-text-primary">TKC não lido em pelo menos um olho: o rastreio de ceratocone (regra 2.1) não pôde ser concluído. Conduta pendente até conferir a tela do Pentacam; nenhuma LIO é indicada automaticamente.</p>
          </div>
        )}
        {tkcStatus === 'classified' && (
          <div className="mx-auto mt-6 max-w-[760px] rounded-xl border border-warning/50 bg-warning-soft p-5" role="status">
            <span className="text-xs font-bold tracking-[0.12em] text-warning">REGRA 2.1</span>
            <p className="mb-0 mt-2 text-sm leading-relaxed text-text-primary">Regra 2.1: TKC classificado — cascata encerrada; encaminhamento a especialista em córnea (avaliação para anel intraestromal). Nenhuma LIO é indicada automaticamente.</p>
          </div>
        )}
</>
        )}
      </div>

      {(decision.state === 'evaluated' || decision.state === 'keratoconus') && (
      <div className="mt-6 rounded-xl border border-primary bg-surface px-5 py-5" data-testid="protocol-orientation">
        <span className="text-xs font-bold tracking-[0.12em] text-primary">
          ORIENTAÇÃO À DECISÃO
        </span>
        {decision.showOrientationSentence && (
          <p className="mb-0 mt-2 text-sm font-bold leading-relaxed text-text-primary">
            Caso compatível, do ponto de vista corneano e biométrico, com a cirurgia indicada abaixo. Avaliar os demais aspectos clínicos e oftalmológicos, bem como a preferência do cirurgião, para dar seguimento.
          </p>
        )}

        <div className="mt-4 grid grid-cols-2 gap-3 max-[720px]:grid-cols-1">
          {eyes.map(({ eye, evaluated }) => (
            <div className={clsx('rounded-lg border p-3', toneCard[evaluated.tone])} data-testid={`conduta-${eye}`} key={`final-verdict-${eye}`}>
              <span className="text-xs font-bold text-text-secondary">{eyeLabel(eye)}</span>
              <strong className="mt-1 block text-sm text-text-primary">{evaluated.conduta}</strong>
              {evaluated.conduta === CONDUTA.readyGeneric && (
                <span className="mt-1 block text-xs text-text-secondary">Escolha entre versão tórica e não tórica depende do astigmatismo da biometria óptica (dado pendente).</span>
              )}
              {evaluated.certainty && (
                <span className="mt-1 block text-[11px] font-bold uppercase tracking-[0.08em] text-text-secondary">
                  {evaluated.certainty === 'pendente' ? 'Conduta pendente' : evaluated.certainty === 'condicional' ? 'Conduta condicional' : 'Conduta definitiva pelo protocolo'}
                </span>
              )}
              {evaluated.review && (
                <span className="mt-1 block text-xs font-bold text-warning" data-testid={`revisar-${eye}`}>{evaluated.review}</span>
              )}
              {evaluated.steps.length > 0 && (
                <ol className="mb-0 mt-2 grid list-none gap-1 p-0 text-xs leading-relaxed text-text-secondary" data-testid={`trilha-${eye}`}>
                  {evaluated.steps.map((step, index) => (
                    <li key={index}>
                      <strong className={clsx(step.outcome === 'fail' && 'text-danger', (step.outcome === 'pending' || step.outcome === 'warn') && 'text-warning', (step.outcome === 'ok' || step.outcome === 'info') && 'text-text-primary')}>{step.rule}</strong> · {step.text}
                    </li>
                  ))}
                </ol>
              )}
            </div>
          ))}
        </div>

        {decision.state === 'evaluated' && (
          <p className="mb-0 mt-3 text-xs leading-relaxed text-text-secondary" data-testid="dados-ausentes">
            <strong>Dados ausentes:</strong>{' '}
            {eyes.flatMap(({ eye, evaluated }) => evaluated.missing.map((item) => `${eyeLabel(eye)}: ${item}`)).join(' · ') || 'Nenhum critério decisivo deste fluxo ficou pendente.'}
          </p>
        )}

        {eyes.some(({ evaluated }) => evaluated.observations.length > 0) && (
          <ul className="mb-0 mt-3 grid list-none gap-2 p-0 text-xs leading-relaxed text-text-secondary" data-testid="observacoes">
            {eyes.flatMap(({ eye, evaluated }) => evaluated.observations.map((observation) => ({ eye, observation })))
              // the same text in both eyes is shown once, as AO (e.g. the rule 2.1 referral)
              .filter(({ observation }, index, all) => all.findIndex((other) => other.observation.text === observation.text) === index)
              .map(({ eye, observation }) => (
                <li className="rounded-lg border border-border bg-surface-muted p-3" key={`${eye}-${observation.type}`}>
                  <strong className="text-text-primary">{eyes.every(({ evaluated }) => evaluated.observations.some((o) => o.text === observation.text)) ? 'AO' : eyeLabel(eye)}</strong> · {observation.text}
                </li>
              ))}
          </ul>
        )}

        <p className="mb-0 mt-3 text-xs leading-relaxed text-text-muted">
          Apoio à decisão em cirurgia refrativa — a indicação final é do médico cirurgião.
        </p>
      </div>
      )}
    </section>
  )
}
