// Copyright (c) 2017 Uber Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

import * as React from 'react';
import { Button, InputRef } from 'antd';
import _get from 'lodash/get';
import _maxBy from 'lodash/maxBy';
import _values from 'lodash/values';
import { IoArrowBack, IoFileTrayFull, IoChevronForward } from 'react-icons/io5';
import { Link } from 'react-router-dom';

import { Helmet } from 'react-helmet';
import AltViewOptions from './AltViewOptions';
import KeyboardShortcutsHelp from './KeyboardShortcutsHelp';
import SpanGraph from './SpanGraph';
import TracePageSearchBar from './TracePageSearchBar';
import { TUpdateViewRangeTimeFunction, IViewRange, ViewRangeTimeUpdate, ETraceViewType } from '../types';
import LabeledList from '../../common/LabeledList';
import NewWindowIcon from '../../common/NewWindowIcon';
import TraceName from '../../common/TraceName';
import { TNil } from '../../../types';
import { Trace } from '../../../types/trace';
import { formatDatetime, formatDuration } from '../../../utils/date';
import { getTraceLinks } from '../../../model/link-patterns';

import './TracePageHeader.css';
import ExternalLinks from '../../common/ExternalLinks';
import { getTargetEmptyOrBlank } from '../../../utils/config/get-target';
import { Input, Typography, Space } from 'antd';

const { TextArea } = Input;
import { Col, Divider, Row } from 'antd';
import { func } from 'prop-types';

import {useState} from 'react';

const {Text, Title} = Typography;


type TracePageHeaderEmbedProps = {
  canCollapse: boolean;
  clearSearch: () => void;
  focusUiFindMatches: () => void;
  hideMap: boolean;
  hideSummary: boolean;
  linkToStandalone: string;
  nextResult: () => void;
  onArchiveClicked: () => void;
  onSlimViewClicked: () => void;
  onTraceViewChange: (viewType: ETraceViewType) => void;
  prevResult: () => void;
  resultCount: number;
  showArchiveButton: boolean;
  showShortcutsHelp: boolean;
  showStandaloneLink: boolean;
  disableJsonView: boolean;
  showViewOptions: boolean;
  slimView: boolean;
  textFilter: string | TNil;
  toSearch: string | null;
  trace: Trace;
  viewType: ETraceViewType;
  updateNextViewRangeTime: (update: ViewRangeTimeUpdate) => void;
  updateViewRangeTime: TUpdateViewRangeTimeFunction;
  viewRange: IViewRange;
};

export const HEADER_ITEMS = [
  {
    key: 'timestamp',
    label: 'Trace Start',
    renderer: (trace: Trace) => {
      const dateStr = formatDatetime(trace.startTime);
      const match = dateStr.match(/^(.+)(\.\d+)$/);
      return match ? (
        <span className="TracePageHeader--overviewItem--value">
          {match[1]}
          <span className="TracePageHeader--overviewItem--valueDetail">{match[2]}</span>
        </span>
      ) : (
        dateStr
      );
    },
  },
  {
    key: 'duration',
    label: 'Duration',
    renderer: (trace: Trace) => formatDuration(trace.duration),
  },
  {
    key: 'service-count',
    label: 'Services',
    renderer: (trace: Trace) => new Set(_values(trace.processes).map(p => p.serviceName)).size,
  },
  {
    key: 'depth',
    label: 'Depth',
    renderer: (trace: Trace) => _get(_maxBy(trace.spans, 'depth'), 'depth', 0) + 1,
  },
  {
    key: 'span-count',
    label: 'Total Spans',
    renderer: (trace: Trace) => trace.spans.length,
  },
];

export function TracePageHeaderFn(props: TracePageHeaderEmbedProps & { forwardedRef: React.Ref<InputRef> }) {
  const {
    canCollapse,
    clearSearch,
    focusUiFindMatches,
    forwardedRef,
    hideMap,
    hideSummary,
    linkToStandalone,
    nextResult,
    onArchiveClicked,
    onSlimViewClicked,
    onTraceViewChange,
    prevResult,
    resultCount,
    showArchiveButton,
    showShortcutsHelp,
    showStandaloneLink,
    showViewOptions,
    disableJsonView,
    slimView,
    textFilter,
    toSearch,
    trace,
    viewType,
    updateNextViewRangeTime,
    updateViewRangeTime,
    viewRange,
  } = props;

  console.log('viewType', viewType)

  if (!trace) {
    return null;
  }

  const links = getTraceLinks(trace);

  const summaryItems =
    !hideSummary &&
    !slimView &&
    HEADER_ITEMS.map(item => {
      const { renderer, ...rest } = item;
      return { ...rest, value: renderer(trace) };
    });

  const traceShortID = trace.traceID.slice(0, 7);

  const title = (
    <h1 className={`TracePageHeader--title ${canCollapse ? 'is-collapsible' : ''}`}>
      <TraceName traceName={trace.traceName} /> <small className="u-tx-muted">{traceShortID}</small>
    </h1>
  );

  const [question, setQuestion] = useState('');
  const [hop, setHop] = useState('1');
  const [conversations, setConversations] = useState([
    {
      type: 'System',
      content: 'Ask away...'
    },
  ]);
  const [loading, setLoading] = useState(false);

  async function onAsk() {
    conversations.push({content: question, type: 'User'})
    setConversations(conversations);

    setLoading(true);

    const response = await fetch("http://localhost:54320/api/ask", {
      body: JSON.stringify({ question, hop: Number(hop) , trace_id: trace.traceID, method: 'graph-rag' }),
      method: 'POST',
    });

    const body = await response.json();
    setLoading(false);

    conversations.push({content: body.answer, type: 'System'})
    setConversations(conversations);

    console.log(body)
  }

  return (
    <header className="TracePageHeader">
      <Helmet title={`${trace.traceEmoji} ${traceShortID}: ${trace.tracePageTitle} — Jaeger UI`} />
      <div className="TracePageHeader--titleRow">
        {toSearch && (
          <Link className="TracePageHeader--back" to={toSearch}>
            <IoArrowBack />
          </Link>
        )}
        {links && links.length > 0 && <ExternalLinks links={links} />}
        {canCollapse ? (
          <a
            className="TracePageHeader--titleLink"
            onClick={onSlimViewClicked}
            role="switch"
            aria-checked={!slimView}
          >
            <IoChevronForward className={`TracePageHeader--detailToggle ${!slimView ? 'is-expanded' : ''}`} />
            {title}
          </a>
        ) : (
          title
        )}
        <TracePageSearchBar
          clearSearch={clearSearch}
          focusUiFindMatches={focusUiFindMatches}
          nextResult={nextResult}
          prevResult={prevResult}
          ref={forwardedRef}
          resultCount={resultCount}
          textFilter={textFilter}
          navigable={viewType === ETraceViewType.TraceTimelineViewer}
        />
        {showShortcutsHelp && <KeyboardShortcutsHelp className="ub-m2" />}
        {showViewOptions && (
          <AltViewOptions
            disableJsonView={disableJsonView}
            onTraceViewChange={onTraceViewChange}
            traceID={trace.traceID}
            viewType={viewType}
          />
        )}
        {showArchiveButton && (
          <Button className="ub-mr2 ub-flex ub-items-center" htmlType="button" onClick={onArchiveClicked}>
            <IoFileTrayFull className="TracePageHeader--archiveIcon" />
            Archive Trace
          </Button>
        )}
        {showStandaloneLink && (
          <Link
            className="u-tx-inherit ub-nowrap ub-mx2"
            to={linkToStandalone}
            target={getTargetEmptyOrBlank()}
            rel="noopener noreferrer"
          >
            <NewWindowIcon isLarge />
          </Link>
        )}
      </div>
      {summaryItems && <LabeledList className="TracePageHeader--overviewItems" items={summaryItems} />}
      {!hideMap && !slimView && (
        <SpanGraph
          trace={trace}
          viewRange={viewRange}
          updateNextViewRangeTime={updateNextViewRangeTime}
          updateViewRangeTime={updateViewRangeTime}
        />
      )}

      <div className='chatSection' style={{padding: '10px', height: '200px', overflow: 'scroll'}}>
        <div className='title' style={{position: 'fixed'}}><Title level={4}>GenAI powered Q&A</Title></div>

        <Row gutter={16} style={{height: 'inherit'}}>
          <Col className="gutter-row" span={10}>
            <div className='ask' style={{position: 'fixed', marginTop: '40px'}}>
              <textarea rows={4}  cols={50}
                        onChange={(e) => setQuestion(e.target.value)}
                        style={{marginBottom: '10px'}} /> <br/>
              <label>Hops: </label>
              <select name='hop' id='' defaultValue={hop}
                      onChange={(e) => setHop(e.target.value)}>
                <option value='0'>0</option>
                <option value='1'>1</option>
                <option value='2'>2</option>
                <option value='3'>3</option>
              </select>
              <button onClick={onAsk} disabled={loading}>Ask</button>
            </div>

          </Col>

          <Col className={'gutter-row'} span={14}>
            <Space direction={'vertical'}>
              {conversations.map(c => (
                 <Text><strong>{c.type}</strong> {c.content}</Text>
              ))}
            </Space>
          </Col>
        </Row>



      </div>
    </header>
  );
}

export default React.forwardRef((props: TracePageHeaderEmbedProps, ref: React.Ref<InputRef>) => (
  <TracePageHeaderFn {...props} forwardedRef={ref} />
));
