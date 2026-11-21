import { createContext, useContext, useState, type FormEvent } from 'react'
import { answerRequest, type Answer } from '../api'
import type { FormField, PendingRequest, Question, RequestPayload } from '../types'
import { IconButton } from './Icon'
import { allowSessionKey, sendKey } from '../keys'

interface RequestCardProps {
  conversationId: string
  request: PendingRequest
  // keyed: this is the approval the reply box's keys answer (allowKeys), so
  // its Allow buttons name them.
  keyed?: boolean
  // held: why the card cannot be answered now, or null. while the page is
  // stale it cannot stand behind the pending set it shows, so the card
  // waits, saying why, as the panel's other controls do.
  held?: string | null
  // stopping: why the answers that would let work go on wait, while an
  // interrupt or a release is under way, or null. Deny and Dismiss stay live,
  // since they only settle the request toward the stop.
  stopping?: string | null
}

// allowKeys are the keys that answer the oldest docked approval from an
// empty reply box or the page itself, as its buttons do: Allow, and "Allow
// this tool for the session" where the harness offers it. a question, a pi
// dialog among them, has no key.
const allowKeys = { allow: sendKey.label, allow_session: allowSessionKey.label }
const ariaKeys = { allow: sendKey.aria, allow_session: allowSessionKey.aria }

// AllowButton is an Allow button; keyed, its title names its key. the key
// presses this same button (InputPanel), so the answer goes the one way.
function AllowButton({ decision, keyed, sending, onAnswer }: { decision: 'allow' | 'allow_session'; keyed: boolean; sending: boolean; onAnswer: (answer: Answer) => void }) {
  const stopping = useContext(Stopping)
  const label = decision === 'allow' ? 'Allow' : 'Allow this tool for the session'
  return (
    <IconButton
      icon={decision === 'allow' ? 'check' : 'done_all'}
      label={label}
      title={keyed ? `${label} (${allowKeys[decision]})` : label}
      aria-keyshortcuts={keyed ? ariaKeys[decision] : undefined}
      word={label}
      data-answer={decision}
      disabled={sending || stopping}
      onClick={() => onAnswer({ decision })}
    />
  )
}

// RequestCard is answerable only because the service's pending set lists the
// request. the first answer from any browser wins; a later one is refused.
// Stopping is whether the card's answers that would let work go on wait for a
// stop under way; Deny and Dismiss read none of it.
const Stopping = createContext(false)

export function RequestCard({ conversationId, request, keyed = false, held = null, stopping = null }: RequestCardProps) {
  const [error, setError] = useState<string | null>(null)
  const [answering, setSending] = useState(false)
  // the forms disable their answers while one is being sent, and while held.
  const sending = answering || held !== null

  const send = async (answer: Answer) => {
    setSending(true)
    setError(null)
    try {
      await answerRequest(conversationId, request.id, answer)
    } catch (failure) {
      setError(failure instanceof Error ? failure.message : String(failure))
      setSending(false)
    }
  }

  // a pi dialog offers Dismiss: pi's own cancel, as Escape is in pi's
  // terminal, sent only when the operator presses it.
  const dismiss = request.payload?.dismiss ? () => void send({ decision: 'dismiss' }) : undefined
  return (
    <Stopping.Provider value={stopping !== null}>
      <section className="request-card" data-request={request.id} aria-label={request.kind === 'question' ? 'Question' : 'Approval'}>
        {request.kind === 'question' && request.payload?.fields ? (
          <FormQuestion payload={request.payload} sending={sending} onAnswer={(answers) => void send({ answers })} onDismiss={dismiss} />
        ) : request.kind === 'question' ? (
          <QuestionForm
            questions={request.payload?.questions ?? []}
            fromPi={!!request.payload?.dialog}
            sending={sending}
            onAnswer={(answers) => void send({ answers })}
            onDismiss={dismiss}
          />
        ) : (
          <PermissionForm request={request} keyed={keyed} sending={sending} onAnswer={(answer) => void send(answer)} />
        )}
        {held && (
          <p className="muted request-held" role="status">
            {held}
          </p>
        )}
        {!held && stopping && (
          <p className="muted request-stopping" role="status">
            {stopping}
          </p>
        )}
        {error && <p className="error">{error}</p>}
      </section>
    </Stopping.Provider>
  )
}

function describeInput(input: Record<string, unknown> | undefined): string {
  if (!input) {
    return ''
  }
  for (const key of ['command', 'file_path', 'path', 'url', 'pattern']) {
    if (typeof input[key] === 'string') {
      return input[key]
    }
  }
  return JSON.stringify(input, null, 2)
}

function PermissionForm({ request, keyed, sending, onAnswer }: { request: PendingRequest; keyed: boolean; sending: boolean; onAnswer: (answer: Answer) => void }) {
  const [message, setMessage] = useState('')
  const payload = request.payload ?? {}
  if (payload.mcp_server) {
    return <McpApproval payload={payload} keyed={keyed} sending={sending} onAnswer={onAnswer} />
  }
  const detail = payload.title ?? payload.description ?? payload.decision_reason
  return (
    <>
      <h2>Approval: {payload.display_name ?? payload.tool ?? 'tool'}</h2>
      {payload.permission_mode === 'auto' && (
        <p className="muted">Raised in auto mode: the classifier could not decide this call, so it needs your answer.</p>
      )}
      {detail && <p>{detail}</p>}
      <pre>{describeInput(payload.input)}</pre>
      {payload.blocked_path && (
        <p className="muted">
          Path: <code>{payload.blocked_path}</code>
        </p>
      )}
      <div className="inline-form">
        <input placeholder="Reason, sent with a denial" value={message} onChange={(event) => setMessage(event.target.value)} />
        <AllowButton decision="allow" keyed={keyed} sending={sending} onAnswer={onAnswer} />
        <IconButton icon="block" label="Deny" word="Deny" disabled={sending} onClick={() => onAnswer({ decision: 'deny', message })} />
      </div>
    </>
  )
}

// McpApproval is an MCP tool approval: the server and tool, what the tool
// does, and the parameters it would be called with. codex carries no reason
// with a denial, so none is asked for. "for this session" appears only where
// the harness offered it, and covers this tool only.
function McpApproval({ payload, keyed, sending, onAnswer }: { payload: RequestPayload; keyed: boolean; sending: boolean; onAnswer: (answer: Answer) => void }) {
  const parameters = payload.parameters ?? []
  return (
    <>
      <h2>Approval: {payload.tool ?? 'MCP tool'}</h2>
      <p className="muted">
        MCP server <code>{payload.mcp_server}</code> wants to run its tool <code>{payload.mcp_tool || 'unknown'}</code>.
      </p>
      {payload.description && <p>{payload.description}</p>}
      {parameters.length > 0 ? (
        <dl className="mcp-parameters">
          {parameters.map((parameter) => (
            <div key={parameter.name}>
              <dt>{parameter.display_name ?? parameter.name}</dt>
              <dd>
                <pre>{typeof parameter.value === 'string' ? parameter.value : JSON.stringify(parameter.value, null, 2)}</pre>
              </dd>
            </div>
          ))}
        </dl>
      ) : (
        payload.input && Object.keys(payload.input).length > 0 && <pre>{JSON.stringify(payload.input, null, 2)}</pre>
      )}
      <div className="actions">
        <AllowButton decision="allow" keyed={keyed} sending={sending} onAnswer={onAnswer} />
        {payload.allow_session && <AllowButton decision="allow_session" keyed={keyed} sending={sending} onAnswer={onAnswer} />}
        <IconButton icon="block" label="Deny" word="Deny" disabled={sending} onClick={() => onAnswer({ decision: 'deny' })} />
      </div>
    </>
  )
}

// FormQuestion is a typed form an MCP server asked for, rendered field by
// field: a choice for an enum, a checkbox for a boolean, a number or text
// input otherwise. every answer travels as text; the adapter types it.
function FormQuestion({
  payload,
  sending,
  onAnswer,
  onDismiss,
}: {
  payload: RequestPayload
  sending: boolean
  onAnswer: (answers: Record<string, string>) => void
  onDismiss?: () => void
}) {
  const fields = payload.fields ?? []
  const [values, setValues] = useState<Record<string, string>>(() => {
    const initial: Record<string, string> = {}
    for (const field of fields) {
      if (field.type === 'boolean') {
        initial[field.name] = String(field.default === true)
      } else if (field.default !== undefined && field.default !== null) {
        initial[field.name] = String(field.default)
      }
    }
    return initial
  })
  const set = (name: string, value: string) => setValues((current) => ({ ...current, [name]: value }))
  const complete = fields.every((field) => !field.required || (values[field.name] ?? '').trim() !== '')

  const onSubmit = (event: FormEvent) => {
    event.preventDefault()
    if (!complete) {
      return
    }
    const answers: Record<string, string> = {}
    for (const field of fields) {
      answers[field.name] = (values[field.name] ?? '').trim()
    }
    onAnswer(answers)
  }

  return (
    <form onSubmit={onSubmit}>
      <h2>
        Question{payload.mcp_server && <> from <code>{payload.mcp_server}</code></>}
        {payload.dialog && ' from a pi extension'}
      </h2>
      {payload.message && <p>{payload.message}</p>}
      {fields.map((field) => (
        <FormFieldInput key={field.name} field={field} value={values[field.name] ?? ''} onChange={(value) => set(field.name, value)} />
      ))}
      <div className="actions">
        <AnswerButton disabled={sending || !complete} />
        {onDismiss && <DismissButton sending={sending} onDismiss={onDismiss} />}
      </div>
    </form>
  )
}

// AnswerButton submits a question's answer, which would let work go on, so it
// waits while a stop is under way.
function AnswerButton({ disabled }: { disabled: boolean }) {
  const stopping = useContext(Stopping)
  return <IconButton type="submit" icon="reply" label="Answer" word="Answer" disabled={disabled || stopping} />
}

function FormFieldInput({ field, value, onChange }: { field: FormField; value: string; onChange: (value: string) => void }) {
  const label = (
    <>
      {field.title ?? field.name}
      {field.required && <span className="muted"> (required)</span>}
      {field.description && <span className="muted"> {field.description}</span>}
    </>
  )
  if (field.type === 'enum') {
    return (
      <fieldset>
        <legend>{label}</legend>
        {(field.options ?? []).map((option) => (
          <label key={option.value} className="option">
            <input type="radio" name={field.name} checked={value === option.value} onChange={() => onChange(option.value)} />
            <span>{option.label}</span>
          </label>
        ))}
      </fieldset>
    )
  }
  if (field.type === 'boolean') {
    return (
      <label className="option">
        <input type="checkbox" checked={value === 'true'} onChange={(event) => onChange(String(event.target.checked))} />
        <span>{label}</span>
      </label>
    )
  }
  if (field.multiline) {
    return (
      <label className="form-field">
        <span>{label}</span>
        <textarea rows={5} value={value} onChange={(event) => onChange(event.target.value)} />
      </label>
    )
  }
  return (
    <label className="form-field">
      <span>{label}</span>
      <input
        type={field.type === 'string' ? 'text' : 'number'}
        step={field.type === 'integer' ? 1 : 'any'}
        value={value}
        onChange={(event) => onChange(event.target.value)}
      />
    </label>
  )
}

// DismissButton sends a pi dialog's own cancel: the extension gets no answer,
// as when Escape is pressed in pi's terminal.
function DismissButton({ sending, onDismiss }: { sending: boolean; onDismiss: () => void }) {
  return (
    <IconButton
      icon="close"
      label="Dismiss: send pi's cancel, as Escape does in pi's terminal"
      word="Dismiss"
      disabled={sending}
      onClick={onDismiss}
    />
  )
}

// QuestionForm is a question with choices. a pi dialog takes only the
// choices it offers, so it has no answer of one's own.
function QuestionForm({
  questions,
  fromPi = false,
  sending,
  onAnswer,
  onDismiss,
}: {
  questions: Question[]
  fromPi?: boolean
  sending: boolean
  onAnswer: (answers: Record<string, string>) => void
  onDismiss?: () => void
}) {
  const [chosen, setChosen] = useState<Record<string, string[]>>({})
  const [other, setOther] = useState<Record<string, string>>({})

  const toggle = (question: Question, label: string) => {
    setChosen((current) => {
      const picked = current[question.question] ?? []
      if (!question.multiSelect) {
        return { ...current, [question.question]: [label] }
      }
      return {
        ...current,
        [question.question]: picked.includes(label) ? picked.filter((l) => l !== label) : [...picked, label],
      }
    })
  }

  const answers: Record<string, string> = {}
  for (const question of questions) {
    const typed = other[question.question]?.trim()
    const picked = chosen[question.question] ?? []
    const value = typed ? [...picked, typed] : picked
    if (value.length > 0) {
      answers[question.question] = value.join(', ')
    }
  }
  const complete = questions.length > 0 && questions.every((question) => answers[question.question])

  const onSubmit = (event: FormEvent) => {
    event.preventDefault()
    if (complete) {
      onAnswer(answers)
    }
  }

  return (
    <form onSubmit={onSubmit}>
      <h2>Question{fromPi && ' from a pi extension'}</h2>
      {questions.map((question) => (
        <fieldset key={question.question}>
          <legend>
            {question.header && <span className="tag">{question.header}</span>} {question.question}
          </legend>
          {(question.options ?? []).map((option) => (
            <label key={option.label} className="option">
              <input
                type={question.multiSelect ? 'checkbox' : 'radio'}
                name={question.question}
                checked={(chosen[question.question] ?? []).includes(option.label)}
                onChange={() => toggle(question, option.label)}
              />
              <span>
                {option.label}
                {option.description && <span className="muted"> {option.description}</span>}
              </span>
            </label>
          ))}
          {!fromPi && (
            <input
              placeholder="Or write your own answer"
              value={other[question.question] ?? ''}
              onChange={(event) => setOther((current) => ({ ...current, [question.question]: event.target.value }))}
            />
          )}
        </fieldset>
      ))}
      <div className="actions">
        <AnswerButton disabled={sending || !complete} />
        {onDismiss && <DismissButton sending={sending} onDismiss={onDismiss} />}
      </div>
    </form>
  )
}
