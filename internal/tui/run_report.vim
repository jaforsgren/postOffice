" Highlighting and folds for the postOffice workflow run report.
" Sourced by vim/nvim with -S after the report is loaded; it is a snapshot,
" so the buffer is made read-only.
scriptencoding utf-8

setlocal readonly nomodifiable nowrap
" Each step's log lines (indented by six spaces) fold under the step:
" zM shows just the steps, zR opens everything, za toggles one step.
setlocal foldmethod=expr foldlevel=99
let &l:foldexpr = "getline(v:lnum) =~# '^      ' ? 1 : 0"

syntax clear

syntax match poRunTitle       /\%1l.*/ contains=poRunWorkflowStatus
syntax match poRunWorkflowStatus /\[\a\+\]$/ contained
syntax match poRunHeading     /^\S.*:$/
syntax match poRunRunError    /^Error:.*/

syntax match poRunStepPass    /^  ✓ .*/ contains=poRunStepId,poRunCodeOk,poRunCodeBad,poRunDuration
syntax match poRunStepFail    /^  ✗ .*/ contains=poRunStepId,poRunCodeOk,poRunCodeBad,poRunDuration
syntax match poRunStepSkip    /^  [−○■] .*/
syntax match poRunStepRunning /^  ▶ .*/ contains=poRunStepId,poRunDuration
syntax match poRunStepId      /^  \S \zs\S\+/ contained
syntax match poRunCodeOk      /\s\zs[123]\d\d\( \u\a*\)*\ze\s\s/ contained
syntax match poRunCodeBad     /\s\zs[45]\d\d\( \u\a*\)*\ze\s\s/ contained
syntax match poRunDuration    /\s\zs[0-9.]\+\(ns\|µs\|ms\|s\|m\)\+$/ contained

syntax match poRunTestPass    /^      ✓ .*/
syntax match poRunTestFail    /^      ✗ [^:]*/ nextgroup=poRunTestReason
syntax match poRunTestReason  /:.*/ contained
syntax match poRunScriptError /^\s*script error:.*/
syntax match poRunErrorTag    /^\s*\zs\[[^\]]*\(ERROR\|FAIL\|STOP\)\]/
syntax match poRunSkipTag     /^\s*\zs\[SKIP\]/
syntax match poRunWarnTag     /^\s*\zs\[warn\]/
syntax match poRunVarTag      /^\s*\zs\[\(collection\|environment\|globals\?\)\]/ nextgroup=poRunVarName skipwhite
syntax match poRunVarName     /\S\+\ze = / contained
syntax match poRunLogTag      /^\s*\zs\[\(log\|info\|debug\)\]/

highlight default poRunPass    ctermfg=2 guifg=#98c379
highlight default poRunFail    ctermfg=1 guifg=#e06c75
highlight default poRunWarn    ctermfg=3 guifg=#e5c07b

highlight default link poRunTitle          Title
highlight default link poRunWorkflowStatus Special
highlight default link poRunHeading        Statement
highlight default link poRunRunError       poRunFail
highlight default link poRunStepPass       poRunPass
highlight default link poRunStepFail       poRunFail
highlight default link poRunStepSkip       Comment
highlight default link poRunStepRunning    poRunWarn
highlight default link poRunStepId         Identifier
highlight default link poRunCodeOk         poRunPass
highlight default link poRunCodeBad        poRunFail
highlight default link poRunDuration       Number
highlight default link poRunTestPass       poRunPass
highlight default link poRunTestFail       poRunFail
highlight default link poRunTestReason     WarningMsg
highlight default link poRunScriptError    poRunFail
highlight default link poRunErrorTag       poRunFail
highlight default link poRunSkipTag        Comment
highlight default link poRunWarnTag        poRunWarn
highlight default link poRunVarTag         Type
highlight default link poRunVarName        Identifier
highlight default link poRunLogTag         Comment
