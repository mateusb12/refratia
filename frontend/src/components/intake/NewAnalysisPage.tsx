import { type ReactNode } from 'react'
import clsx from 'clsx'
import { ArrowLeft, Check, Database, UploadCloud } from 'lucide-react'
import ExamProtocolChecklist from './ExamProtocolChecklist'
import IntakeCheckpointPreflight from './IntakeCheckpointPreflight'
import IntakeFileClassification from './IntakeFileClassification'
import IntakeProcessingProgress from './IntakeProcessingProgress'
import { IntakeAnalysisSummary, IntakeCompletenessNotice, IntakeDocumentsDebug, PatientChangePreview } from './IntakePreview'
import { useIntakeWorkflow } from './useIntakeWorkflow'
import { formatBirthDate } from '../../utils/date'

interface SavedCase {
  caseId: string
  patientName: string
}

function formatCaseSavedTime(caseId: string) {
  const match = caseId.match(/^case-(\d{8})T(\d{2})(\d{2})\d{2}Z-/)
  if (!match) return null
  const localHour = (Number(match[2]) + 21) % 24
  return `${String(localHour).padStart(2, '0')}h${match[3]}`
}

const steps = [
  { title: 'Enviar exames', description: 'Carregar o caso' },
  { title: 'Conferir documentos', description: 'Exames recebidos' },
  { title: 'Conferir dados', description: 'Tipo e olho dos arquivos' },
  { title: 'Analisar arquivos', description: 'Processamento e OCR' },
  { title: 'Revisar relatório', description: 'Decisão médica' },
] as const

function Eyebrow({ children }: { children: ReactNode }) {
  return <span className="text-primary text-xs font-bold tracking-[0.13em]">{children}</span>
}

function StatusBadge({ children, tone }: { children: ReactNode; tone: 'success' | 'warning' | 'blocking' | 'neutral' }) {
  return <span className={clsx('inline-flex w-fit items-center gap-1.5 rounded-full px-2.5 py-1.5 text-xs font-bold', tone === 'success' && 'bg-success-soft text-success', tone === 'warning' && 'bg-warning-soft text-warning', tone === 'blocking' && 'bg-danger-soft text-danger', tone === 'neutral' && 'bg-surface-muted text-text-secondary')}>{children}</span>
}

function PrimaryButton({ children, onClick, disabled }: { children: ReactNode; onClick?: () => void; disabled?: boolean }) {
  return <button className="inline-flex items-center justify-center gap-2 rounded-[9px] border-0 bg-primary px-4 py-[11px] text-sm font-semibold text-white transition-[background,transform] duration-150 hover:enabled:-translate-y-px hover:enabled:bg-primary-hover disabled:cursor-not-allowed disabled:opacity-45" disabled={disabled} onClick={onClick} type="button">{children}</button>
}

function ProcessSteps({ current, reviewed, reachedStep = current, maxStep = steps.length, onStepChange }: { current: number; reviewed: boolean; reachedStep?: number; maxStep?: number; onStepChange?: (step: number) => void }) {
  return <ol className="mt-6 grid list-none grid-cols-5 gap-2 p-0 max-[1100px]:grid-cols-2 max-[580px]:grid-cols-1" aria-label="Etapas da análise">
    {steps.map((step, index) => {
      const number = index + 1
      const completed = (number !== current && number <= reachedStep) || (number === steps.length && reviewed)
      const active = current === number && !completed
      const available = number <= maxStep
      return <li key={step.title} className="min-w-0">
        <button className={clsx('flex w-full min-w-0 items-center gap-2.5 rounded-[10px] border p-[11px] text-left', active && 'border-primary-border bg-primary-soft', completed && 'border-border bg-surface-muted', !active && !completed && 'border-border bg-surface opacity-60', available && onStepChange && 'cursor-pointer hover:border-primary', !available && 'cursor-not-allowed')} disabled={!available || !onStepChange} onClick={() => onStepChange?.(number)} type="button">
          <span className={clsx('grid h-7 w-7 flex-none place-items-center rounded-full border text-xs font-bold', active && 'border-primary bg-primary text-white', completed && 'border-success bg-success text-white', !active && !completed && 'border-border-strong bg-surface text-text-secondary')}>{completed ? <Check size={14} strokeWidth={3} /> : number}</span>
          <span className="min-w-0"><strong className="block truncate text-xs">{step.title}</strong><small className="mt-0.5 block truncate text-xs text-text-muted">{step.description}</small></span>
        </button>
      </li>
    })}
  </ol>
}

export interface NewAnalysisPageProps {
  apiUrl: string
  visible: boolean
  selectedCase: 'demo' | 'real' | null
  processingStep: number
  reportData: unknown
  isReviewed: boolean
  savedCases: SavedCase[]
  selectedCaseId: string | null
  onCaseConfirmed: (newCase: SavedCase) => void
  onNavigateToReports: () => void
  onBackToReports: () => void
  onOpenCase: (caseId: string) => void
}

export default function NewAnalysisPage({
  apiUrl,
  visible,
  selectedCase,
  processingStep,
  reportData,
  isReviewed,
  savedCases,
  selectedCaseId,
  onCaseConfirmed,
  onNavigateToReports,
  onBackToReports,
  onOpenCase,
}: NewAnalysisPageProps) {
  const intake = useIntakeWorkflow({ apiUrl, onCaseConfirmed, onNavigateToReports })
  const isRealCase = selectedCase === 'real'
  const workflowStep = reportData || isRealCase
    ? steps.length
    : selectedCase === 'demo'
      ? processingStep
      : intake.intakePreview
        ? 4
        : intake.intakeFiles.length > 0
          ? intake.intakeWorkflowStep
          : 1
  const workflowStatus = reportData || isRealCase
    ? (isReviewed ? 'Relatório revisado' : 'Revise o relatório')
    : intake.intakePreview
      ? 'Análise concluída'
      : intake.intakeFiles.length > 0
        ? intake.intakeWorkflowStep >= 4
          ? 'Analisando arquivos'
          : intake.intakeWorkflowStep >= 3 ? 'Confira os dados' : 'Confira os documentos'
        : 'Envie os exames'
  const reportGenerated = workflowStep === steps.length || Boolean(reportData)

  const {
    intakeBusy,
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
    discardIntake,
    goToIntakeStep,
    replaceIntakeFiles,
    setIntakeClassifications,
    setIntakeFiles,
    setIntakeForceReprocess,
    setIntakeMessage,
    setIntakeProcessFiles,
    setIntakePreview,
  } = intake

  return (
            <div className={visible ? undefined : 'hidden'}>
              {selectedCase && (
                <button
                  className="mb-4 inline-flex items-center gap-2 border-0 bg-transparent p-0 text-sm font-semibold text-primary hover:text-primary-hover"
                  onClick={onBackToReports}
                  type="button"
                >
                  <ArrowLeft size={16} /> Voltar para casos
                </button>
              )}
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
                    maxStep={reportData || isRealCase
                      ? steps.length
                      : intakeWorkflowStep >= 4 || intakeBusy || intakePreview
                        ? 4
                        : intakeFiles.length > 0
                          ? 3
                          : 1}
                    reachedStep={reportData || isRealCase
                      ? steps.length
                      : selectedCase === 'demo'
                        ? workflowStep
                        : intakeReachedStep}
                    onStepChange={(step) => {
                      if (!reportData && !isRealCase) goToIntakeStep(step)
                    }}
                    reviewed={isReviewed}
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
                            workflowStep >= 3 && (
                              intakeBusy ||
                              intakeCheckpointBusy ||
                              !intakeClassificationComplete
                            )
                          }
                          onClick={() => {
                            if (workflowStep < 3) {
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
                    {workflowStep < 3 && <ExamProtocolChecklist files={intakeFiles} />}

                    {workflowStep === 3 && <>
                    <IntakeFileClassification
                      apiUrl={apiUrl}
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

                    {intakeClassificationComplete && (
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
                    <IntakeCompletenessNotice analysis={intakePreview.analysis} />
                    <IntakeAnalysisSummary analysis={intakePreview.analysis} />
                    <details className="mt-4 rounded-lg border border-border bg-surface">
                      <summary className="cursor-pointer px-3 py-2 text-xs font-bold text-primary">Ver JSON completo (debug)</summary>
                      <pre className="m-0 max-h-[520px] overflow-auto border-t border-border p-3 text-xs leading-relaxed text-text-secondary">{JSON.stringify(intakePreview.analysis, null, 2)}</pre>
                    </details>
                    <div className="mt-4 flex flex-wrap gap-3">
                      <PrimaryButton disabled={intakeBusy} onClick={confirmIntake}>
                        {intakeBusy
                          ? 'Salvando…'
                          : intakePreview.patientMatch.status === 'existing'
                            ? 'Atualizar paciente'
                            : intakePreview.patientMatch.status === 'new'
                              ? 'Criar paciente'
                              : 'Revisar identidade'}
                      </PrimaryButton>
                      <button className="rounded-[9px] border border-border-strong bg-surface px-4 py-[11px] text-sm font-semibold hover:border-danger hover:text-danger" disabled={intakeBusy} onClick={discardIntake} type="button">Descartar</button>
                    </div>
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
                  <StatusBadge tone={reportGenerated ? 'success' : 'neutral'}>
                    {reportGenerated ? 'Pronto para revisar' : 'Aguardando caso'}
                  </StatusBadge>
                </div>
                <div className="mt-5 grid gap-4">
                  {savedCases.map((item) => (
                    <button
                      aria-pressed={selectedCaseId === item.caseId}
                      className={clsx(
                        'grid grid-cols-[auto_minmax(0,1fr)_auto] items-center rounded-xl border bg-surface-muted p-[18px] text-left hover:border-primary hover:bg-primary-soft/80',
                        selectedCaseId === item.caseId ? 'border-primary ring-2 ring-primary/15' : 'border-border-strong',
                      )}
                      key={item.caseId}
                      onClick={() => void onOpenCase(item.caseId)}
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

