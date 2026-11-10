// every icon in the ui: material icons from @material-design-icons/svg,
// pinned exactly, each imported as raw svg text so it is embedded in the
// build. no icon font, nothing fetched at runtime. the svg text is ours, from
// the package at build time, never message content.
import addComment from '@material-design-icons/svg/filled/add_comment.svg?raw'
import arrowDownward from '@material-design-icons/svg/filled/arrow_downward.svg?raw'
import block from '@material-design-icons/svg/filled/block.svg?raw'
import check from '@material-design-icons/svg/filled/check.svg?raw'
import close from '@material-design-icons/svg/filled/close.svg?raw'
import contentCopy from '@material-design-icons/svg/filled/content_copy.svg?raw'
import createNewFolder from '@material-design-icons/svg/filled/create_new_folder.svg?raw'
import deleteIcon from '@material-design-icons/svg/filled/delete.svg?raw'
import doneAll from '@material-design-icons/svg/filled/done_all.svg?raw'
import download from '@material-design-icons/svg/filled/download.svg?raw'
import edit from '@material-design-icons/svg/filled/edit.svg?raw'
import errorOutline from '@material-design-icons/svg/filled/error_outline.svg?raw'
import logout from '@material-design-icons/svg/filled/logout.svg?raw'
import moreVert from '@material-design-icons/svg/filled/more_vert.svg?raw'
import playArrow from '@material-design-icons/svg/filled/play_arrow.svg?raw'
import reply from '@material-design-icons/svg/filled/reply.svg?raw'
import rocketLaunch from '@material-design-icons/svg/filled/rocket_launch.svg?raw'
import send from '@material-design-icons/svg/filled/send.svg?raw'
import stop from '@material-design-icons/svg/filled/stop.svg?raw'
import undo from '@material-design-icons/svg/filled/undo.svg?raw'
import unfoldLess from '@material-design-icons/svg/filled/unfold_less.svg?raw'
import unfoldMore from '@material-design-icons/svg/filled/unfold_more.svg?raw'
import visibility from '@material-design-icons/svg/filled/visibility.svg?raw'
import type { ButtonHTMLAttributes } from 'react'

const icons = {
  add_comment: addComment,
  arrow_downward: arrowDownward,
  block,
  check,
  close,
  content_copy: contentCopy,
  create_new_folder: createNewFolder,
  delete: deleteIcon,
  done_all: doneAll,
  download,
  edit,
  error_outline: errorOutline,
  logout,
  more_vert: moreVert,
  play_arrow: playArrow,
  reply,
  rocket_launch: rocketLaunch,
  send,
  stop,
  undo,
  unfold_less: unfoldLess,
  unfold_more: unfoldMore,
  visibility,
}

export type IconName = keyof typeof icons

// Icon is a material icon inlined from its bundled svg, filled with the text
// colour around it. it is decoration: its button carries the label.
export function Icon({ name }: { name: IconName }) {
  return <span className="icon" aria-hidden="true" dangerouslySetInnerHTML={{ __html: icons[name] }} />
}

// IconButton is a button whose meaning is its icon: the label is its title
// and its accessible name, so it is never unnamed. with a word, the word sits
// beside the icon and the label still names the action.
export function IconButton({
  icon,
  label,
  word,
  className,
  ...rest
}: { icon: IconName; label: string; word?: string } & ButtonHTMLAttributes<HTMLButtonElement>) {
  return (
    <button type="button" title={label} aria-label={label} className={`icon-button${word ? ' with-word' : ''}${className ? ` ${className}` : ''}`} {...rest}>
      <Icon name={icon} />
      {word && <span className="word">{word}</span>}
    </button>
  )
}
