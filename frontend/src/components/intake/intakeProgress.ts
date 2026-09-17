export type IntakeFileProcessingStatus =
  | 'waiting'
  | 'processing'
  | 'extracted'
  | 'identified'
  | 'failed'

export interface IntakeFileProgressState {
  percent: number
  status: IntakeFileProcessingStatus
  stage: string
  message: string
  examType?: string
  eye?: string
}

export interface IntakeStreamEvent {
  type?: string
  percent?: number
  filePercent?: number
  stage?: string
  message?: string
  filename?: string
  status?: number
  payload?: unknown
}

interface IntakeFileResultPayload {
  filename?: string
  examType?: string
  eye?: string
  status?: string
  message?: string
}

function clampProgress(value: number) {
  return Math.max(
    0,
    Math.min(
      100,
      Math.round(value),
    ),
  )
}

function isFinalStatus(
  status: IntakeFileProcessingStatus,
) {
  return (
    status === 'extracted' ||
    status === 'identified' ||
    status === 'failed'
  )
}

function isFileProcessingStatus(
  value: unknown,
): value is IntakeFileProcessingStatus {
  return (
    value === 'waiting' ||
    value === 'processing' ||
    value === 'extracted' ||
    value === 'identified' ||
    value === 'failed'
  )
}

export function createInitialIntakeFileProgress(
  files: File[],
): Record<string, IntakeFileProgressState> {
  return Object.fromEntries(
    files.map((file) => [
      file.name,
      {
        percent: 0,
        status: 'waiting',
        stage: '',
        message: 'Aguardando processamento',
      } satisfies IntakeFileProgressState,
    ]),
  )
}

export function updateIntakeFileProgress(
  current: Record<string, IntakeFileProgressState>,
  event: IntakeStreamEvent,
): Record<string, IntakeFileProgressState> {
  const result =
    event.payload &&
    typeof event.payload === 'object'
      ? event.payload as IntakeFileResultPayload
      : undefined

  const filename =
    event.filename ||
    result?.filename

  if (!filename) {
    return current
  }

  const previous =
    current[filename] ?? {
      percent: 0,
      status: 'waiting',
      stage: '',
      message: 'Aguardando processamento',
    }

  if (event.type === 'progress') {
    if (isFinalStatus(previous.status)) {
      return current
    }

    return {
      ...current,
      [filename]: {
        ...previous,
        percent:
          typeof event.filePercent === 'number'
            ? clampProgress(event.filePercent)
            : previous.percent,
        status: 'processing',
        stage:
          event.stage ??
          previous.stage,
        message:
          event.message ??
          previous.message,
      },
    }
  }

  if (event.type !== 'file_result') {
    return current
  }

  const status =
    isFileProcessingStatus(result?.status)
      ? result.status
      : 'identified'

  return {
    ...current,
    [filename]: {
      ...previous,
      percent: 100,
      status,
      stage:
        result?.examType ||
        event.stage ||
        previous.stage,
      message:
        result?.message ||
        event.message ||
        previous.message,
      examType:
        result?.examType ||
        previous.examType,
      eye:
        result?.eye ||
        previous.eye,
    },
  }
}
