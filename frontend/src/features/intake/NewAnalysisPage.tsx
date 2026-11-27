import clsx from 'clsx'
import { Check, Database, ScanText, Sparkles, UploadCloud } from 'lucide-react'
import { formatCaseSavedTime, type SavedCase } from '../../shared/cases'
import { Eyebrow, PrimaryButton, StatusBadge } from '../../shared/ui'
import ExamProtocolChecklist from './ExamProtocolChecklist'
import { hasCompleteOpticalBiometry } from './examProtocol'
import IntakeCheckpointPreflight from './IntakeCheckpointPreflight'
import IntakeFileClassification from './IntakeFileClassification'
import IntakeProcessingProgress from './IntakeProcessingProgress'
import { IntakeAnalysisSummary, IntakeDocumentsDebug, PatientChangePreview } from './IntakePreview'
import { useIntakeWorkflow } from './useIntakeWorkflow'
import { formatBirthDate } from '../../shared/date'

const steps = [
  { title: 'Enviar exames', description: 'Carregar o caso' },
  { title: 'Conferir documentos', description: 'Exames recebidos' },
  { title: 'Conferir dados', description: 'Tipo e olho dos arquivos' },
  { title: 'Analisar arquivos', description: 'Processamento e OCR' },
  { title: 'Revisar relatório', description: 'Decisão médica' },
] as const

function ProcessSteps({ current, reachedStep, maxStep, onStepChange }: { current: number; reachedStep: number; maxStep: number; onStepChange: (step: number) => void }) {
  return <ol className="mt-6 grid list-none grid-cols-5 gap-2 p-0 max-[1100px]:grid-cols-2 max-[580px]:grid-cols-1" aria-label="Etapas da análise">
    {steps.map((step, index) => {
      const number = index + 1
      const completed = number !== current && number <= reachedStep
      const active = current === number && !completed
      const available = number <= maxStep
      return <li key={step.title} className="min-w-0">
        <button className={clsx('flex w-full min-w-0 items-center gap-2.5 rounded-[10px] border p-[11px] text-left', active && 'border-primary-border bg-primary-soft', completed && 'border-border bg-surface-muted', !active && !completed && 'border-border bg-surface opacity-60', available && 'cursor-pointer hover:border-primary', !available && 'cursor-not-allowed')} disabled={!available} onClick={() => onStepChange(number)} type="button">
          <span className={clsx('grid h-7 w-7 flex-none place-items-center rounded-full border text-xs font-bold', active && 'border-primary bg-primary text-white', completed && 'border-success bg-success text-white', !active && !completed && 'border-border-strong bg-surface text-text-secondary')}>{completed ? <Check size={14} strokeWidth={3} /> : number}</span>
          <span className="min-w-0"><strong className="block truncate text-xs">{step.title}</strong><small className="mt-0.5 block truncate text-xs text-text-muted">{step.description}</small></span>
        </button>
      </li>
    })}
  </ol>
}

export default function NewAnalysisPage({
  visible,
  cases,
  onCaseConfirmed,
  onOpenCase,
}: {
  visible: boolean
  cases: SavedCase[]
  onCaseConfirmed: (newCase: SavedCase) => void
  onOpenCase: (caseId: string) => void
}) {
  const intake = useIntakeWorkflow(onCaseConfirmed)
  const workflowStep = intake.intakePreview
    ? 4
    : intake.intakeFiles.length > 0
      ? intake.intakeWorkflowStep
      : 1
  const workflowStatus = intake.intakePreview
    ? 'Análise concluída'
    : intake.intakeFiles.length > 0
      ? intake.intakeWorkflowStep >= 4
        ? 'Analisando arquivos'
        : intake.intakeWorkflowStep >= 3 ? 'Confira os dados' : 'Confira os documentos'
      : 'Envie os exames'
  const opticalBiometryComplete = (() => {
    const eyes = new Set(
      Object.values(intake.intakeClassifications)
        .filter((item) => item.examType === 'iol_calculation')
        .map((item) => item.eye),
    )
    const classifiedBiometryComplete = eyes.has('AO') || (eyes.has('OD') && eyes.has('OS'))

    // Keep the filename signal while the async inspection is still filling in
    // the classifications. The individual file classifications are still
    // required by `intakeClassificationComplete` before analysis.
    return classifiedBiometryComplete || hasCompleteOpticalBiometry(intake.intakeFiles)
  })()

  const {
    intakeBusy,
    identityReview,
    intakeCheckpointBusy,
    intakeCheckpointError,
    intakeCheckpointStates,
    intakeClassificationComplete,
    intakeClassifications,
    intakeCompletedFiles,
    intakeElapsed,
    intakeFileProgress,
    intakeFiles,
    intakeForceReprocess,
    intakeLiveAnalysis,
    intakeLocalPreviews,
    intakeMessage,
    intakePreview,
    intakeProcessFiles,
    intakeProgress,
    intakeProgressFilename,
    intakeProgressMessage,
    intakeProgressStage,
    intakeReachedStep,
    intakeWorkflowStep,
    analyzeIntake,
    abortIntakeAnalysis,
    confirmIntake,
    deleteDraft,
    goToIntakeStep,
    replaceIntakeFiles,
    setIntakeClassifications,
    setIntakeFiles,
    setIntakeForceReprocess,
    setIntakeMessage,
    setIntakeProcessFiles,
    setIntakePreview,
    claudeIdentity,
    claudeIdentityComplete,
    claudeStatus,
    extractionEngine,
    setClaudeIdentity,
    setExtractionEngine,
  } = intake
  const claudeSelected = extractionEngine === 'claude'
  const engineLabel = claudeSelected
    ? `Claude Vision — Demonstração${claudeStatus?.mode === 'fixture' ? ' (resposta gravada, sem chamada à API)' : claudeStatus?.model ? ` (${claudeStatus.model})` : ''}`
    : 'OCR atual (Tesseract)'

  return (
            <div className={visible ? undefined : 'hidden'}>
              <section className="mt-5 rounded-2xl border border-primary-border bg-primary-soft/30 p-6 shadow-sm max-[580px]:p-4">
                <Eyebrow>{workflowStep >= 3 ? 'CONFERIR DADOS' : 'IMPORTAR EXAMES'}</Eyebrow>
                <h2 className="mb-0 mt-1 font-display text-xl">
                  {workflowStep >= 3 ? 'Confirme os dados dos arquivos' : 'Envie os arquivos do paciente'}
                </h2>
                <p className="mb-0 mt-1.5 text-sm leading-relaxed text-text-secondary">
                  {workflowStep >= 3
                    ? 'Revise o tipo e o olho de cada arquivo antes de iniciar a análise.'
                    : 'Após a análise, o sistema identifica o paciente e mostra exatamente o que será criado, adicionado ou alterado antes da confirmação.'}
                </p>
                <div className="mt-5 rounded-xl border border-border bg-surface/60 p-4">
                  <div className="flex items-center justify-between gap-3">
                    <div>
                      <Eyebrow>ETAPA ATUAL</Eyebrow>
                      <p className="mb-0 mt-1 text-sm font-semibold text-text-primary">{workflowStatus}</p>
                    </div>
                    <StatusBadge tone={workflowStep === 2 ? 'warning' : workflowStep >= 4 ? 'success' : 'neutral'}>
                      Etapa {Math.min(workflowStep, steps.length)} de {steps.length}
                    </StatusBadge>
                  </div>
                  <ProcessSteps
                    current={workflowStep}
                    maxStep={intakeWorkflowStep >= 4 || intakeBusy || intakePreview
                        ? 4
                        : intakeFiles.length > 0
                          ? 3
                          : 1}
                    reachedStep={intakeReachedStep}
                    onStepChange={goToIntakeStep}
                  />
                </div>
                {workflowStep < 3 && <label
                  className="mt-5 flex cursor-pointer flex-col items-center justify-center rounded-xl border-2 border-dashed border-primary/40 bg-surface p-8 text-center hover:border-primary max-[580px]:p-6"
                  onDragOver={(event) => event.preventDefault()}
                  onDrop={(event) => {
                    event.preventDefault()
                    void deleteDraft(intakePreview)
                    replaceIntakeFiles(Array.from(event.dataTransfer.files))
                    setIntakePreview(null)
                    setIntakeMessage('')
                  }}
                >
                  <UploadCloud className="text-primary" size={30} />
                  <strong className="mt-3 text-sm">Arraste os arquivos aqui ou clique para selecionar</strong>
                  <span className="mt-1 text-xs text-text-muted">PDF, DOC, DOCX, XLS, XLSX, JPG e PNG · até 20 MB por arquivo</span>
                  <input
                    accept=".pdf,.doc,.docx,.xls,.xlsx,.jpg,.jpeg,.png"
                    className="sr-only"
                    multiple
                    onChange={(event) => {
                      void deleteDraft(intakePreview)
                      replaceIntakeFiles(Array.from(event.target.files ?? []))
                      setIntakePreview(null)
                      setIntakeMessage('')
                    }}
                    type="file"
                  />
                </label>}
                {intakeFiles.length > 0 && (
                  <div className="mt-4 rounded-xl border border-border bg-surface p-4">
                    <div className="flex flex-wrap items-center justify-between gap-3">
                      <strong className="text-sm">
                        {intakeFiles.length} arquivo(s) {workflowStep >= 4 ? 'em análise' : 'selecionado(s)'}
                      </strong>
                      <div className="flex items-center gap-2">
                        {intakeBusy && (
                          <button
                            className="rounded-xl border border-danger/40 bg-danger-soft px-4 py-2.5 text-sm font-bold text-danger hover:border-danger/70"
                            onClick={abortIntakeAnalysis}
                            type="button"
                          >
                            Cancelar análise
                          </button>
                        )}

                        {workflowStep < 4 && <PrimaryButton
                          disabled={
                            workflowStep < 3
                              ? !opticalBiometryComplete
                              : intakeBusy ||
                                intakeCheckpointBusy ||
                                !intakeClassificationComplete ||
                                !opticalBiometryComplete ||
                                (claudeSelected && (!claudeStatus?.available || !claudeIdentityComplete))
                          }
                          onClick={() => {
                            if (workflowStep < 3) {
                              if (!opticalBiometryComplete) {
                                setIntakeMessage('Envie a biometria óptica (EyeSuite) completa — AO ou OD e OS — antes de continuar para a análise.')
                                return
                              }
                              goToIntakeStep(3)
                              return
                            }
                            void analyzeIntake()
                          }}
                        >
                          {workflowStep < 3
                            ? 'Próxima etapa'
                            : intakeBusy
                            ? 'Analisando…'
                            : intakeCheckpointBusy
                              ? 'Verificando arquivos…'
                              : intakeForceReprocess
                                ? 'Reprocessar arquivos'
                                : 'Analisar arquivos'}
                        </PrimaryButton>}
                      </div>
                    </div>
                    {workflowStep === 3 && (
                    <fieldset className="mt-4 rounded-xl border border-border bg-surface-muted/40 p-4" disabled={intakeBusy}>
                      <legend className="px-1 text-xs font-bold uppercase tracking-[0.12em] text-text-secondary">Motor de extração</legend>
                      <div className="flex flex-wrap gap-3" role="radiogroup" aria-label="Motor de extração">
                        {([
                          ['ocr', 'OCR atual', ScanText, true],
                          ['claude', 'Claude Vision — Demonstração', Sparkles, Boolean(claudeStatus?.available)],
                        ] as const).map(([value, label, Icon, enabled]) => (
                          <button
                            aria-checked={extractionEngine === value}
                            className={clsx(
                              'flex h-32 w-40 flex-col items-center justify-center gap-3 rounded-xl border p-3 text-center text-xs font-bold',
                              extractionEngine === value ? 'border-primary bg-primary-soft text-primary' : 'border-border bg-surface text-text-secondary hover:border-primary',
                              !enabled && 'cursor-not-allowed opacity-50',
                            )}
                            disabled={!enabled}
                            key={value}
                            onClick={() => setExtractionEngine(value)}
                            role="radio"
                            type="button"
                          >
                            {/* ponytail: lucide placeholder; swap for an <img> when the pictures exist */}
                            <Icon size={48} strokeWidth={1.5} />
                            {label}
                          </button>
                        ))}
                      </div>
                      {!claudeStatus?.available && (
                        <p className="mb-0 mt-2 text-xs text-warning" role="status">
                          Claude Vision indisponível: {claudeStatus?.reason ?? 'verificando configuração do backend…'}
                        </p>
                      )}
                      {claudeSelected && (
                        <div className="mt-3 grid gap-3">
                          <p className="m-0 text-xs text-text-secondary">
                            Somente PDFs gerados e liberados por <code>frontend/scripts/anonymize_exams.py</code> são aceitos; documentos originais são recusados pelo backend.
                            {claudeStatus?.mode === 'fixture' && ' Modo fixture: a resposta gravada do Claude é reproduzida sem chamar a API.'}
                          </p>
                          <p className="m-0 text-xs text-text-secondary">
                            Identificação do paciente (contexto local autorizado; não é enviada ao Claude):
                          </p>
                          <div className="grid gap-3 sm:grid-cols-2">
                            <label className="grid gap-1 text-xs font-semibold">
                              Nome completo
                              <input
                                className="rounded-lg border border-border bg-surface px-3 py-2 text-sm"
                                onChange={(event) => setClaudeIdentity((current) => ({ ...current, fullName: event.target.value }))}
                                value={claudeIdentity.fullName}
                              />
                            </label>
                            <label className="grid gap-1 text-xs font-semibold">
                              Data de nascimento
                              <input
                                className="rounded-lg border border-border bg-surface px-3 py-2 text-sm"
                                onChange={(event) => setClaudeIdentity((current) => ({ ...current, birthDate: event.target.value }))}
                                type="date"
                                value={claudeIdentity.birthDate}
                              />
                            </label>
                          </div>
                        </div>
                      )}
                    </fieldset>
                    )}
                    {workflowStep < 3 && <ExamProtocolChecklist files={intakeFiles} />}

                    {workflowStep < 4 && !opticalBiometryComplete && (
                      <p className="mb-0 mt-3 rounded-lg border border-danger/30 bg-danger-soft p-3 text-sm font-semibold text-danger" role="alert">
                        A análise está bloqueada: a biometria óptica (EyeSuite) é obrigatória. Envie um arquivo AO ou os arquivos OD e OS para liberar a próxima etapa.
                      </p>
                    )}

                    {workflowStep === 3 && <>
                    <IntakeFileClassification
                      files={intakeFiles}
                      localPreviews={intakeLocalPreviews}
                      onChange={setIntakeClassifications}
                      onRemove={(file) => {
                        void deleteDraft(intakePreview)
                        setIntakeFiles((files) => files.filter((currentFile) => currentFile !== file))
                        setIntakePreview(null)
                      }}
                      value={intakeClassifications}
                    />

                    {!intakeClassificationComplete && (
                      <p className="mb-0 mt-3 text-xs text-warning">
                        Confirme o tipo e o olho de todos os arquivos para liberar a verificação de processamento.
                      </p>
                    )}


                    {intakeClassificationComplete && !claudeSelected && (
                      <IntakeCheckpointPreflight
                        checking={intakeCheckpointBusy}
                        disabled={intakeBusy}
                        error={intakeCheckpointError}
                        files={intakeFiles}
                        forceReprocess={intakeForceReprocess}
                        onProcessFilesChange={(key, value) => {
                          setIntakeProcessFiles((current) => ({
                            ...current,
                            [key]: value,
                          }))
                        }}
                        onForceReprocessChange={setIntakeForceReprocess}
                        processFiles={intakeProcessFiles}
                        states={intakeCheckpointStates}
                      />
                    )}
                    </>}

                    {workflowStep >= 4 && intakeBusy && (
                      <p className="mb-0 mt-4 text-xs font-bold text-text-secondary" data-testid="extraction-engine">Motor: {engineLabel}</p>
                    )}
                    {workflowStep >= 4 && intakeBusy && (
                      <IntakeProcessingProgress
                        elapsedSeconds={intakeElapsed}
                        fileProgress={intakeFileProgress}
                        files={intakeFiles}
                        overallFilename={intakeProgressFilename}
                        overallMessage={intakeProgressMessage}
                        overallProgress={intakeProgress}
                        overallStage={intakeProgressStage}
                      />
                    )}
                    {workflowStep >= 4 && intakeLiveAnalysis && (
                      <div className="mt-4 rounded-xl border border-success/40 bg-success-soft p-4">
                        <div className="flex flex-wrap items-start justify-between gap-3">
                          <div>
                            <span className="text-xs font-bold uppercase tracking-[0.12em] text-success">
                              Resultados durante a análise
                            </span>

                            <p className="mb-0 mt-1 text-xs text-text-secondary">
                              Exames exibidos assim que cada parser termina.
                            </p>
                          </div>

                          <span className="rounded-full border border-success/30 bg-surface px-3 py-1 text-xs font-bold text-success">
                            {intakeCompletedFiles.length} processado(s)
                          </span>
                        </div>

                        {intakeCompletedFiles.length > 0 && (
                          <div className="mt-4 grid gap-2 sm:grid-cols-2">
                            {intakeCompletedFiles.map((filename) => (
                              <div
                                className="flex min-w-0 items-center gap-3 rounded-lg border border-success/20 bg-surface/70 px-3 py-2"
                                key={filename}
                              >
                                <span className="grid h-6 w-6 flex-none place-items-center rounded-full bg-success-soft text-success">
                                  ✓
                                </span>

                                <div className="min-w-0">
                                  <strong className="block text-xs text-success">
                                    Parsing concluído
                                  </strong>

                                  <span
                                    className="block truncate font-mono text-[10px] text-text-secondary"
                                    title={filename}
                                  >
                                    {filename}
                                  </span>
                                </div>
                              </div>
                            ))}
                          </div>
                        )}

                        <div className="mt-4 border-t border-success/20 pt-4">
                          <IntakeAnalysisSummary
                            analysis={intakeLiveAnalysis}
                          />
                        </div>
                      </div>
                    )}
                  </div>
                )}
                {intakePreview && (
                  <div className="mt-4 rounded-xl border border-success/30 bg-success-soft p-4">
                    <div className="flex flex-wrap items-start justify-between gap-3">
                      <div>
                        <strong className="text-sm">
                          {intakePreview.patientMatch.status === 'existing'
                            ? 'Paciente já cadastrado'
                            : intakePreview.patientMatch.status === 'new'
                              ? 'Novo paciente'
                              : 'Identidade precisa de revisão'}
                        </strong>
                        <p className="mb-0 mt-1 text-sm font-semibold text-text-primary">
                          {intakePreview.patientMatch.status === 'existing'
                            ? intakePreview.patientMatch.patientName
                              ?? intakePreview.analysis.patient?.full_name
                              ?? 'Paciente não identificado'
                            : intakePreview.analysis.patient?.full_name
                              ?? 'Paciente não identificado'}
                        </p>
                        <p className="mb-0 mt-1 text-xs text-text-secondary">
                          {formatBirthDate(intakePreview.analysis.patient?.birth_date)}
                          {' · '}
                          {Object.keys(intakePreview.analysis.exams ?? {}).length} tipo(s) de exame
                          {intakePreview.patientMatch.status === 'existing'
                            ? ` · ${intakePreview.patientMatch.caseId}`
                            : ''}
                        </p>
                      </div>
                      <StatusBadge tone="success">
                        {intakePreview.patientMatch.status === 'existing'
                          ? 'Atualização'
                          : intakePreview.patientMatch.status === 'new'
                            ? 'Novo cadastro'
                            : 'Revisar identidade'}
                      </StatusBadge>
                    </div>

                    <p className="mb-0 mt-2 text-sm leading-relaxed text-text-secondary">
                      {intakePreview.patientMatch.status === 'existing'
                        ? 'Os exames enviados serão incorporados ao registro existente somente após sua confirmação.'
                        : intakePreview.patientMatch.status === 'new'
                          ? 'Este paciente ainda não possui um caso correspondente e será criado somente após sua confirmação.'
                          : 'Não foi possível determinar com segurança se este paciente já possui cadastro. Revise a identidade antes de confirmar.'}
                    </p>

                    <PatientChangePreview preview={intakePreview} />
                    <IntakeDocumentsDebug localPreviews={intakeLocalPreviews} preview={intakePreview} />
                    <IntakeAnalysisSummary analysis={intakePreview.analysis} showOverview={false} showFallbackWarning={false} />
                    <details className="mt-4 rounded-lg border border-border bg-surface">
                      <summary className="cursor-pointer px-3 py-2 text-xs font-bold text-primary">Ver JSON completo (debug)</summary>
                      <pre className="m-0 max-h-[520px] overflow-auto border-t border-border p-3 text-xs leading-relaxed text-text-secondary">{JSON.stringify(intakePreview.analysis, null, 2)}</pre>
                    </details>
                    <div className="mt-5">
                      <PrimaryButton className="min-h-16 w-full max-w-md !bg-[#287f69] px-8 py-4 text-xl font-extrabold tracking-wide hover:enabled:!bg-[#1f6d5a]" disabled={intakeBusy} onClick={() => confirmIntake()}>
                        {intakeBusy
                          ? 'Salvando…'
                          : intakePreview.patientMatch.status === 'existing'
                            ? 'Atualizar paciente'
                            : intakePreview.patientMatch.status === 'new'
                              ? 'Criar paciente'
                              : 'Revisar identidade'}
                      </PrimaryButton>
                    </div>
                    {identityReview && (
                      <div className="mt-4 rounded-lg border border-warning/40 bg-warning-soft p-4 text-sm" role="alert">
                        <p className="m-0 font-bold">Já existe paciente com a mesma data de nascimento e nome parecido. É a mesma pessoa?</p>
                        {identityReview.map((candidate) => (
                          <div key={candidate.caseId} className="mt-2 flex flex-wrap items-center gap-3">
                            <span>{candidate.name} · {candidate.caseId}</span>
                            <button className="rounded border border-border px-3 py-1 font-bold" disabled={intakeBusy} onClick={() => confirmIntake({ action: 'existing', caseId: candidate.caseId })} type="button">Mesma pessoa: incorporar</button>
                          </div>
                        ))}
                        <button className="mt-3 rounded border border-border px-3 py-1 font-bold" disabled={intakeBusy} onClick={() => confirmIntake({ action: 'new' })} type="button">Pessoa diferente: criar novo paciente</button>
                      </div>
                    )}
                  </div>
                )}
                {intakeMessage && <p aria-live="assertive" className="mb-0 mt-3 whitespace-pre-wrap rounded-lg border border-danger/30 bg-danger-soft p-3 text-sm font-semibold text-danger">{intakeMessage}</p>}
              </section>
              <section className="mt-5 rounded-2xl border border-border bg-surface p-6 shadow-sm max-[580px]:p-4">
                <div className="flex items-start justify-between gap-4">
                  <div>
                    <Eyebrow>FLUXO DA ANÁLISE</Eyebrow>
                    <h2 className="mb-0 mt-1 font-display text-xl">Escolha um caso para revisar</h2>
                  </div>
                  <StatusBadge tone="neutral">Aguardando caso</StatusBadge>
                </div>
                <div className="mt-5 grid gap-4">
                  {cases.map((item) => (
                    <button
                      className="grid grid-cols-[auto_minmax(0,1fr)_auto] items-center rounded-xl border border-border-strong bg-surface-muted p-[18px] text-left hover:border-primary hover:bg-primary-soft/80"
                      key={item.caseId}
                      onClick={() => onOpenCase(item.caseId)}
                      type="button"
                    >
                      <span className="grid h-[45px] w-[45px] place-items-center rounded-xl bg-success-soft text-success">
                        <Database size={22} />
                      </span>
                      <span className="mx-3.5 min-w-0">
                        <strong className="block truncate text-sm">{item.patientName}</strong>
                        <small className="mt-1 block text-xs leading-relaxed text-text-muted">
                          Caso salvo · {formatCaseSavedTime(item.caseId) ?? item.caseId}
                        </small>
                      </span>
                      <StatusBadge tone="success">Abrir relatório</StatusBadge>
                    </button>
                  ))}
                </div>
              </section>
            </div>
)
}

