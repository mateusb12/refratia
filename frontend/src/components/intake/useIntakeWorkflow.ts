import { useEffect, useRef, useState } from 'react'
import { isIntakePreview, type IntakeAnalysis, type IntakePreview } from '../../contracts/patient-analysis'
import { type IntakeClassification } from './IntakeFileClassification'
import {
  intakeFileCheckpointKey,
  inspectIntakeFileCheckpoints,
  type IntakeFileCheckpointState,
} from './intakeCheckpoint'
import {
  createInitialIntakeFileProgress,
  type IntakeFileProgressState,
  type IntakeStreamEvent,
  updateIntakeFileProgress,
} from './intakeProgress'

interface SavedCase {
  caseId: string
  patientName: string
  analysisKey?: string
}

interface UseIntakeWorkflowOptions {
  apiUrl: string
  onCaseConfirmed: (newCase: SavedCase) => void
  onNavigateToReports: () => void
}

export function useIntakeWorkflow({
  apiUrl,
  onCaseConfirmed,
  onNavigateToReports,
}: UseIntakeWorkflowOptions) {
  const [intakeWorkflowStep, setIntakeWorkflowStep] = useState(1)
  const [intakeReachedStep, setIntakeReachedStep] = useState(1)
  const [intakeFiles, setIntakeFiles] = useState<File[]>([])
  const [intakeClassifications, setIntakeClassifications] = useState<Record<string, IntakeClassification>>({})
  const [intakeLocalPreviews, setIntakeLocalPreviews] = useState<Record<string, string>>({})
  const [intakePreview, setIntakePreview] = useState<IntakePreview | null>(null)
  const [intakeBusy, setIntakeBusy] = useState(false)
  const intakeAbortController = useRef<AbortController | null>(null)
  const [intakeProgress, setIntakeProgress] = useState(0)
  const [intakeProgressStage, setIntakeProgressStage] = useState('')
  const [intakeProgressMessage, setIntakeProgressMessage] = useState('')
  const [intakeProgressFilename, setIntakeProgressFilename] = useState('')
  const [intakeLiveAnalysis, setIntakeLiveAnalysis] = useState<IntakeAnalysis | null>(null)
  const [intakeCompletedFiles, setIntakeCompletedFiles] = useState<string[]>([])
  const [intakeFileProgress, setIntakeFileProgress] = useState<Record<string, IntakeFileProgressState>>({})
  const [intakeCheckpointStates, setIntakeCheckpointStates] = useState<Record<string, IntakeFileCheckpointState>>({})
  const [intakeCheckpointBusy, setIntakeCheckpointBusy] = useState(false)
  const [intakeCheckpointError, setIntakeCheckpointError] = useState('')
  const [intakeProcessFiles, setIntakeProcessFiles] = useState<Record<string, boolean>>({})
  const [intakeForceReprocess, setIntakeForceReprocess] = useState(false)
  const intakeCheckpointRequestId = useRef(0)
  const [intakeElapsed, setIntakeElapsed] = useState(0)
  const [intakeMessage, setIntakeMessage] = useState('')

  const intakeClassificationComplete = intakeFiles.length > 0 && Object.values(intakeClassifications)
    .filter((item) => item.examType && item.eye).length === intakeFiles.length

  function goToIntakeStep(step: number) {
    setIntakeWorkflowStep(step)
    setIntakeReachedStep((current) => Math.max(current, step))
  }

  useEffect(() => {
    if (!intakeBusy) return
    const timer = window.setInterval(() => setIntakeElapsed((elapsed) => elapsed + 1), 1000)
    return () => window.clearInterval(timer)
  }, [intakeBusy])

  useEffect(() => {
    const requestId = intakeCheckpointRequestId.current + 1
    intakeCheckpointRequestId.current = requestId
    setIntakeForceReprocess(false)
    setIntakeProcessFiles({})
    setIntakeCheckpointError('')

    if (!intakeFiles.length || !intakeClassificationComplete || !apiUrl) {
      setIntakeCheckpointStates({})
      setIntakeCheckpointBusy(false)
      return
    }

    const controller = new AbortController()
    setIntakeCheckpointBusy(true)

    void inspectIntakeFileCheckpoints(apiUrl, intakeFiles, controller.signal)
      .then((states) => {
        if (intakeCheckpointRequestId.current !== requestId) return
        setIntakeCheckpointStates(states)
        setIntakeProcessFiles(Object.fromEntries(
          Object.entries(states).map(([key, state]) => [key, state.status !== 'processed']),
        ))
      })
      .catch((checkpointError) => {
        if (controller.signal.aborted || intakeCheckpointRequestId.current !== requestId) return
        setIntakeCheckpointStates({})
        setIntakeCheckpointError(
          checkpointError instanceof Error
            ? checkpointError.message
            : 'Não foi possível consultar o histórico dos arquivos.',
        )
      })
      .finally(() => {
        if (intakeCheckpointRequestId.current === requestId) setIntakeCheckpointBusy(false)
      })

    return () => controller.abort()
  }, [apiUrl, intakeClassificationComplete, intakeFiles])

  async function analyzeIntake() {
    if (!intakeFiles.length) return

    if (!intakeClassificationComplete) {
      setIntakeMessage('Confirme o tipo e o olho de cada arquivo antes de analisar.')
      return
    }
    if (!apiUrl) {
      setIntakeMessage('VITE_API_URL não está configurada no .env. Exemplo local: VITE_API_URL=http://localhost:3000')
      return
    }

    const controller = new AbortController()
    intakeAbortController.current = controller
    setIntakeBusy(true)
    goToIntakeStep(4)
    setIntakeProgress(1)
    setIntakeProgressStage('upload')
    setIntakeProgressMessage('Enviando documentos')
    setIntakeProgressFilename('')
    setIntakeLiveAnalysis(null)
    setIntakeCompletedFiles([])
    setIntakeFileProgress(createInitialIntakeFileProgress(intakeFiles))
    setIntakeElapsed(0)
    setIntakeMessage('')

    try {
      const body = new FormData()
      intakeFiles.forEach((file) => body.append('files', file))
      body.append('classifications', JSON.stringify(Object.values(intakeClassifications).filter((item) => item.examType && item.eye)))
      body.append('processFiles', JSON.stringify(
        intakeFiles
          .filter((file) => intakeProcessFiles[intakeFileCheckpointKey(file)])
          .map((file) => intakeCheckpointStates[intakeFileCheckpointKey(file)]?.sha256)
          .filter((sha256): sha256 is string => Boolean(sha256)),
      ))

      const response = await fetch(`${apiUrl}/api/intakes/analyze?stream=1${intakeForceReprocess ? '&force=1' : ''}`, {
        method: 'POST',
        headers: { Accept: 'application/x-ndjson' },
        signal: controller.signal,
        body,
      })
      const contentType = response.headers.get('content-type') ?? ''
      let result: unknown = null

      if (contentType.includes('application/x-ndjson')) {
        if (!response.body) throw new Error('O backend não disponibilizou o stream de progresso.')
        const reader = response.body.getReader()
        const decoder = new TextDecoder()
        let buffer = ''

        const consumeLine = (line: string) => {
          const trimmed = line.trim()
          if (!trimmed) return
          let event: IntakeStreamEvent
          try {
            event = JSON.parse(trimmed)
          } catch {
            throw new Error('O backend enviou um evento de progresso inválido.')
          }
          if (event.filename) setIntakeProgressFilename(event.filename)
          setIntakeFileProgress((current) => updateIntakeFileProgress(current, event))
          if (event.type === 'progress') {
            if (typeof event.percent === 'number') setIntakeProgress(Math.max(0, Math.min(100, Math.round(event.percent))))
            if (event.stage) setIntakeProgressStage(event.stage)
            if (event.message) setIntakeProgressMessage(event.message)
          } else if (event.type === 'partial') {
            if (event.payload && typeof event.payload === 'object' && 'exams' in event.payload) setIntakeLiveAnalysis(event.payload as IntakeAnalysis)
            if (event.filename) setIntakeCompletedFiles((current) => current.includes(event.filename!) ? current : [...current, event.filename!])
          } else if (event.type === 'error') {
            throw new Error(event.message || `Falha na análise${event.status ? ` (HTTP ${event.status})` : ''}`)
          } else if (event.type === 'result') {
            result = event.payload
          }
        }

        while (true) {
          const { value, done } = await reader.read()
          if (done) break
          buffer += decoder.decode(value, { stream: true })
          const lines = buffer.split('\n')
          buffer = lines.pop() ?? ''
          lines.forEach(consumeLine)
        }
        buffer += decoder.decode()
        if (buffer.trim()) consumeLine(buffer)
      } else {
        const responseText = await response.text()
        try {
          result = responseText ? JSON.parse(responseText) : null
        } catch {
          if (!response.ok) throw new Error(`Falha na análise (HTTP ${response.status}): ${responseText.slice(0, 300) || 'o servidor não retornou detalhes'}`)
        }
        if (!response.ok) {
          const errorMessage = result && typeof result === 'object' && 'error' in result && typeof result.error === 'string'
            ? result.error
            : `Falha na análise (HTTP ${response.status})`
          throw new Error(errorMessage)
        }
        setIntakeProgress(100)
      }

      if (!isIntakePreview(result)) throw new Error('A API de análise está desatualizada. Reinicie o backend e tente novamente.')
      setIntakeProgress(100)
      setIntakeProgressStage('complete')
      setIntakeProgressMessage('Análise concluída')
      setIntakePreview(result)
      setIntakeFiles([])
    } catch (error) {
      if (error instanceof Error && error.name === 'AbortError') {
        setIntakeProgressStage('cancelled')
        setIntakeProgressMessage('Análise cancelada')
        setIntakeMessage('Análise cancelada. Os resultados já processados foram preservados.')
      } else if (error instanceof TypeError) {
        setIntakeMessage(`NetworkError: não foi possível conectar ao backend em ${apiUrl}. Verifique se o backend está ativo.`)
      } else {
        setIntakeMessage(error instanceof Error ? error.message : 'Não foi possível analisar os arquivos.')
      }
    } finally {
      if (intakeAbortController.current === controller) intakeAbortController.current = null
      setIntakeBusy(false)
    }
  }

  function abortIntakeAnalysis() {
    const controller = intakeAbortController.current
    if (!controller || controller.signal.aborted) return
    setIntakeProgressMessage('Cancelando análise…')
    controller.abort()
  }

  function replaceIntakeFiles(files: File[]) {
    if (files.length > 0) goToIntakeStep(2)
    setIntakeClassifications({})
    setIntakeFiles((current) => {
      const merged = [...current]
      files.forEach((file) => {
        if (!merged.some((existing) => existing.name === file.name && existing.size === file.size && existing.lastModified === file.lastModified)) merged.push(file)
      })
      return merged
    })
    setIntakeLocalPreviews((current) => {
      const next = { ...current }
      files.forEach((file) => { if (!next[file.name]) next[file.name] = URL.createObjectURL(file) })
      return next
    })
  }

  async function deleteDraft(preview: IntakePreview | null) {
    if (!preview) return true
    try {
      return (await fetch(`${apiUrl}/api/intakes/${encodeURIComponent(preview.intakeId)}`, { method: 'DELETE' })).ok
    } catch {
      return false
    }
  }

  async function confirmIntake() {
    if (!intakePreview) return
    setIntakeBusy(true)
    setIntakeMessage('')
    try {
      const response = await fetch(`${apiUrl}/api/intakes/confirm`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ intakeId: intakePreview.intakeId }),
      })
      const result = await response.json()
      if (!response.ok) throw new Error(result.error ?? 'Falha ao salvar o caso')
      onCaseConfirmed({ caseId: result.caseId, patientName: intakePreview.analysis.patient?.full_name || 'Paciente não identificado', analysisKey: result.analysisKey })
      setIntakeFiles([])
      setIntakeClassifications({})
      setIntakeReachedStep(1)
      setIntakeLocalPreviews((current) => {
        Object.values(current).forEach((url) => URL.revokeObjectURL(url))
        return {}
      })
      setIntakePreview(null)
      onNavigateToReports()
    } catch (error) {
      setIntakeMessage(error instanceof Error ? error.message : 'Não foi possível salvar o caso.')
    } finally {
      setIntakeBusy(false)
    }
  }

  async function discardIntake() {
    setIntakeBusy(true)
    if (await deleteDraft(intakePreview)) {
      setIntakeFiles([])
      setIntakeClassifications({})
      setIntakeReachedStep(1)
      setIntakeLocalPreviews((current) => {
        Object.values(current).forEach((url) => URL.revokeObjectURL(url))
        return {}
      })
      setIntakePreview(null)
      setIntakeMessage('Rascunho descartado do storage.')
    } else {
      setIntakeMessage('Não foi possível descartar o rascunho. Tente novamente.')
    }
    setIntakeBusy(false)
  }

  return {
    intakeAbortController,
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
    setIntakeFiles,
    setIntakeMessage,
    setIntakePreview,
    setIntakeClassifications,
    setIntakeForceReprocess,
    setIntakeProcessFiles,
  }
}

export type IntakeWorkflow = ReturnType<typeof useIntakeWorkflow>
