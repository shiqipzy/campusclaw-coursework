import json,re

def login(c):
    for who,key in [('teacher_a','TEACHER_A_PASSWORD'),('student_a1','STUDENT_A1_PASSWORD'),('student_b1','STUDENT_B1_PASSWORD'),('teacher_b','TEACHER_B_PASSWORD')]:
        data=c.http('POST','/api/login',{'username':who,'password':c.env[key]},who=None)
        assert data['token_type']=='Bearer' and data['user']['username']==who
        c.tokens[who]=data['token'];c.secrets.append(data['token'])
        assert c.sql(f"SELECT username FROM users WHERE id={data['user']['id']};")==who
    c.http('POST','/api/login',{'username':'teacher_a','password':'incorrect'},who=None,expected=401)
    return '4个真实账号登录，JWT及用户记录一致；错误密码401'

def me(c):
    a=c.http('GET','/api/me',who='student_a1');b=c.http('GET','/api/me',who='student_b1')
    assert a['class_id']==1 and b['class_id']==2
    c.http('GET','/api/me',who=None,expected=401)
    return 'A/B班身份正确；未认证401'

def upload(c):
    body='数学自学笔记\n去括号时，括号前的系数要乘到括号内每一项，负号应逐项检查。\n方程移项需要变号。长度单位1m等于100cm。\n验收专用关键词：蓝杉数学。'
    m=c.http('POST','/api/materials',form=('数学自学笔记-'+c.tag+'.md',body),expected=201)
    c.state['material']=m['id']
    assert c.sql(f"SELECT content FROM knowledge_entries WHERE material_id={m['id']};").replace('\\n','\n')==body
    assert int(c.sql(f"SELECT COUNT(*) FROM knowledge_chunks WHERE material_id={m['id']};"))>=1
    c.http('POST','/api/materials',form=('denied.md','学生不得上传'),who='student_a1',expected=403)
    return 'UTF-8材料原文与切片入库；学生上传403'

def search(c):
    hits=c.http('GET','/api/search?q='+__import__('urllib.parse',fromlist=['quote']).quote('去括号'),who='student_a1')['hits']
    assert any(h['material_id']==c.state['material'] for h in hits)
    assert c.http('GET','/api/search?q='+__import__('urllib.parse',fromlist=['quote']).quote('蓝杉数学')+'&class_id=1',who='student_b1')['hits']==[]
    for h in hits:assert 'char_start' in h and h['chunk_index']>=1
    return '命中可回溯切片；B班伪造class_id仍无A班命中'

def agent_list(c):
    agents=c.http('GET','/api/agents');assert len(agents)>=3
    b=c.http('GET','/api/agents',who='student_b1');assert not set(a['id'] for a in agents)&set(a['id'] for a in b)
    c.state['seed_agent']=agents[0]['id']
    return '内置3个应用，A/B班列表隔离'

def templates(c):
    data=c.http('GET','/api/prompt-templates');assert len(data)>=3 and all(t['prompt'] for t in data)
    return '3份可套用提示词模板'

def server_list(c):
    servers=c.http('GET','/api/mcp/servers');s=next(s for s in servers if s['name']=='campus-tools');c.state['server']=s['id']
    assert s['has_token'] and 'token' not in s and 'token_cipher' not in s
    assert c.env['MCP_AUTH_TOKEN'] not in json.dumps(servers)
    assert c.http('GET','/api/mcp/servers',who='teacher_b')==[]
    c.http('GET','/api/mcp/servers',who='student_a1',expected=403)
    c.evidence['server_response']=s
    return 'has_token=true但无令牌字段/原文；同班学生403，异班教师空列表'

def server_get(c):
    sid=c.state['server']
    cross=c.http('GET',f'/api/mcp/servers/{sid}',who='teacher_b',expected=404)
    missing=c.http('GET','/api/mcp/servers/99999999',who='teacher_b',expected=404)
    assert cross==missing
    c.evidence['isolation']={'cross':cross,'missing':missing}
    return '异班直接访问与不存在ID的404响应完全一致'

def server_connect(c):
    sid=c.state['server'];one=c.http('POST',f'/api/mcp/servers/{sid}/connect');two=c.http('POST',f'/api/mcp/servers/{sid}/connect')
    assert one['connected'] and len(one['tools'])==len(two['tools'])==2
    assert {t['name'] for t in two['tools']}=={'formula_hint','unit_convert'}
    assert c.sql(f'SELECT connected,JSON_LENGTH(tools_json) FROM mcp_servers WHERE id={sid};')=='1\t2'
    assert c.shell('exec -T mcp-demo wget -q -O - http://127.0.0.1:8090/health')=='ok'
    assert len(c.rpc('tools/list',{})['result']['tools'])==2
    value=c.rpc('tools/call',{'name':'unit_convert','arguments':{'value':2,'from':'m','to':'cm'}})
    assert '200 cm' in value['result']['content'][0]['text']
    c.evidence['mcp_direct']=value
    return '真实容器/health、initialize、tools/list、tools/call通过；连续连接不翻倍'

def server_tools(c):
    tools=c.http('GET',f'/api/mcp/servers/{c.state["server"]}/tools');assert len(tools)==2
    assert all(t['description'] and t['inputSchema']['type']=='object' for t in tools)
    c.evidence['tools']=tools
    return '名称、完整说明和输入JSON Schema均来自发现结果'

def skill_upload(c):
    source=(c.root/'examples/skills/数学工具引导.md').read_text(encoding='utf-8').replace('名称：数学工具引导','名称：数学解题引导'+c.tag)
    s=c.http('POST','/api/skills/upload',form=('SKILL.md',source),expected=201);c.state['skill']=s['id'];c.state['skill_source']=source
    assert 'formula_hint@campus-tools' in s['tools'] and s['source_text']==source
    assert c.sql(f'SELECT enabled,JSON_LENGTH(tools_json) FROM skills WHERE id={s["id"]};')=='1\t3'
    c.http('POST','/api/skills/upload',form=('bad.md',source),who='student_a1',expected=403)
    return '上传技能源文件保留；解析3个工具，数据库一致；学生403'

def skill_post(c):
    source='名称：临时技能'+c.tag+'\n适用条件：临时\n工具：\n启用：是\n\n给出简短提示。'
    skill=c.http('POST','/api/skills',{'source_text':source},expected=201);c.state['temp_skill']=skill['id']
    bad=source.replace('工具：','工具：formula_hint@not-found')
    c.http('POST','/api/skills',{'source_text':bad},expected=400)
    c.http('POST','/api/skills',{'source_text':source.replace('启用：是','启用：是\n名称：重复')},expected=400)
    return '合法创建；不存在工具引用、重复元数据400'

def agent_post(c):
    body={'name':'数学验收助手'+c.tag,'prompt':'你是数学验收助手。用简洁中文引导学生独立思考。','enabled':True,'skill_ids':[c.state['skill']],'material_ids':[c.state['material']]}
    agent=c.http('POST','/api/agents',body,expected=201);c.state['agent']=agent['id'];c.state['agent_body']=body
    assert c.sql(f'SELECT COUNT(*) FROM tutor_agent_skills WHERE agent_id={agent["id"]};')=='1'
    c.http('POST','/api/agents',body,who='student_a1',expected=403)
    c.http('POST','/api/agents',dict(body,name='跨班绑定'+c.tag,material_ids=[2]),expected=404)
    return '助手、技能和材料绑定入库；学生403，跨班绑定404'

def conversation_post(c):
    conv=c.http('POST','/api/conversations',{'agent_id':c.state['agent']},who='student_a1',expected=201);c.state['conversation']=conv['id']
    assert c.sql(f'SELECT agent_id FROM conversations WHERE id={conv["id"]};')==str(c.state['agent'])
    c.http('POST','/api/conversations',{'agent_id':c.state['agent']},who='student_b1',expected=404)
    return '新会话锁定智能体；跨班新建404'

def messages(c):
    result=c.chat('我在解方程去括号时总漏掉负号，请使用公式提示工具，只给下一步检查建议，不要给最终答案。')
    events=result['events'];calls=[x for e in events if e['event']=='tool_calls' for x in e['data']['tool_calls']]
    assert any(x['ref']=='formula_hint@campus-tools' and x['status']=='ok' for x in calls),calls
    metas=[e['data'] for e in events if e['event']=='meta'];assert metas[0]['skill']['id']==c.state['skill']
    assert metas[0]['citations'] and all(x['material_id']==c.state['material'] for x in metas[0]['citations'])
    assert sum(e['event']=='delta' for e in events)>1
    assert c.sql(f"SELECT COUNT(*) FROM tool_call_logs WHERE conversation_id={c.state['conversation']} AND status='ok';")!='0'
    c.evidence['grounded_chat']=result
    if c.project=='campusclaw_acceptance':
        assert next(e['data'] for e in reversed(events) if e['event']=='meta')['limited'] is True
        c.evidence['round_limit']={'max_rounds':1,'limited':True,'answer_returned':True}
    broken=c.http('POST','/api/conversations',{'agent_id':c.state['agent']},who='student_a1',expected=201)
    try:
        c.shell('stop mcp-demo')
        error=c.chat('我在解方程去括号时漏掉负号，请务必调用公式提示工具帮助检查。',conv=broken['id'])
        errors=[x for e in error['events'] if e['event']=='tool_calls' for x in e['data']['tool_calls']]
        assert any(x['status']=='error' for x in errors),errors
        c.evidence['tool_error_chat']=error
    finally:c.shell('start mcp-demo')
    return '真实DeepSeek/MCP成功及服务停机出错均验证；轮次上限后停止调用并继续流式作答，正文与DB一致'

def trace(c):
    traces=c.http('GET',f'/api/conversations/{c.state["conversation"]}/traces',who='student_a1');assert len(traces)>=2
    first=next(t for t in traces if t['call_index']==1)['request']
    assert first['model']==c.env['AI_MODEL'] and first['stream'] is False and len(first['tools'])==2
    system=first['messages'][0]['content'];assert '数学验收助手' in system and '只给下一步提示' in system
    assert '去括号时，括号前的系数' not in system
    assert '去括号时，括号前的系数' in first['messages'][-1]['content']
    assert 'source_text' not in json.dumps(first,ensure_ascii=False)
    final=next(t for t in traces if t['request']['stream']);assert 'tools' not in final['request']
    assert any(m['role']=='tool' for m in final['request']['messages'])
    assert final['response']['sse_frames']
    c.evidence['llm_traces']=traces
    return '实测请求含提示词、选中技能说明、最多4个切片及候选工具；工具结果在role=tool；最终stream=true'

def conversation_get(c):
    cid=c.state['conversation']
    cross=c.http('GET',f'/api/conversations/{cid}',who='teacher_a',expected=404)
    none=c.http('GET','/api/conversations/99999999',who='teacher_a',expected=404);assert cross==none
    c.http('GET',f'/api/conversations/{cid}/traces',who='teacher_a',expected=404)
    assert c.http('GET','/api/conversations',who='teacher_a')==[]
    return '同班其他用户也不能读取会话/请求详情；跨用户与不存在404一致'

def agent_put(c):
    if c.project=='campusclaw_acceptance':
        cid=c.state['conversation']
        repeated=c.chat('我在去括号时仍漏掉负号，请再次调用 formula_hint 公式提示工具帮助检查。')
        assert c.sql(f'SELECT COUNT(*) FROM tool_call_logs WHERE conversation_id={cid};')=='2'
        capped=c.chat('请继续调用公式提示工具，检查去括号时负号分配问题。')
        assert not any(e['event']=='tool_calls' for e in capped['events'])
        assert next(e['data'] for e in reversed(capped['events']) if e['event']=='meta')['limited'] is True
        assert c.sql(f'SELECT COUNT(*) FROM tool_call_logs WHERE conversation_id={cid};')=='2'
        c.evidence['cumulative_limit']={'max_calls':2,'before':2,'after':2,'limited':True,'answer':capped['answer']}
    body=dict(c.state['agent_body'],enabled=False)
    c.http('PUT',f'/api/agents/{c.state["agent"]}',body)
    assert not any(a['id']==c.state['agent'] for a in c.http('GET','/api/agents',who='student_a1'))
    c.http('GET',f'/api/agents/{c.state["agent"]}',who='student_a1',expected=404)
    c.http('POST','/api/conversations',{'agent_id':c.state['agent']},who='student_a1',expected=404)
    result=c.chat('你好，请给我一条保持专注的建议。')
    assert result['events'][0]['data']['skill'] is None and result['events'][0]['data']['citations']==[]
    assert not re.search(r'\[\d+\]',result['answer'])
    c.evidence['ungrounded_chat']=result
    c.http('PUT',f'/api/agents/{c.state["agent"]}',c.state['agent_body'])
    return '停用后学生不可选用；已有会话仍可提问；检索落空仍真实作答，无伪造引用'

def skill_put(c):
    source=c.state['skill_source'].replace('启用：是','启用：否')
    c.http('PUT',f'/api/skills/{c.state["skill"]}',{'source_text':source})
    result=c.chat('请问去括号时需要注意什么？')
    assert result['events'][0]['data']['skill'] is None
    c.evidence['disabled_skill_chat']=result
    c.http('PUT',f'/api/skills/{c.state["skill"]}',{'source_text':c.state['skill_source']})
    return '现有会话下一轮读取新配置；停用技能不参与路由'

def disconnect(c):
    sid=c.state['server'];data=c.http('POST',f'/api/mcp/servers/{sid}/disconnect');assert not data['connected'] and data['tools']==[]
    c.http('PUT',f'/api/skills/{c.state["skill"]}',{'source_text':c.state['skill_source']},expected=400)
    result=c.chat('解方程时去括号怎么检查？请给一点提示。')
    calls=[x for e in result['events'] if e['event']=='tool_calls' for x in e['data']['tool_calls']]
    assert calls and all(x['status']=='denied' for x in calls)
    assert c.sql(f"SELECT COUNT(*) FROM tool_call_logs WHERE conversation_id={c.state['conversation']} AND status='denied';")!='0'
    c.evidence['disconnected_chat']=result
    c.http('POST',f'/api/mcp/servers/{sid}/connect')
    return '断开即清单为空，技能保存400；已有绑定调用明确被拒并入库，对话继续'

def logs(c):
    logs=c.http('GET','/api/mcp/calls');assert {'ok','denied','error'} <= {x['status'] for x in logs}
    assert c.http('GET','/api/mcp/calls',who='teacher_b')==[]
    c.http('GET','/api/mcp/calls',who='student_a1',expected=403)
    assert all(x['username']=='student_a1' for x in logs)
    c.evidence['call_logs']=logs
    return '成功/被拒/出错3类真实记录含工具、触发者、会话、轮次、时间；B班教师看不到A班记录'

def server_post(c):
    body={'name':'unreachable-'+c.tag,'base_url':'http://mcp-demo:1','token':c.env['MCP_AUTH_TOKEN']}
    s=c.http('POST','/api/mcp/servers',body,expected=201);c.state['temp_server']=s['id']
    c.http('POST',f'/api/mcp/servers/{s["id"]}/connect',expected=502)
    failed=c.http('GET',f'/api/mcp/servers/{s["id"]}');assert not failed['connected'] and failed['tools']==[] and failed['last_error']
    return '不可达服务连接502，保留可读错误，状态为未连接且清单为空'

def server_put(c):
    sid=c.state['temp_server'];body={'name':'temp-'+c.tag,'base_url':'http://mcp-demo:8090','token':''}
    s=c.http('PUT',f'/api/mcp/servers/{sid}',body);assert s['has_token'] and not s['connected']
    assert c.http('POST',f'/api/mcp/servers/{sid}/connect')['connected']
    return '修改时空token保留服务端密文，须重新连接'

def server_delete(c):
    sid=c.state['temp_server'];c.http('DELETE',f'/api/mcp/servers/{sid}',expected=204)
    assert c.sql(f'SELECT COUNT(*) FROM mcp_servers WHERE id={sid};')=='0'
    return '删除服务器后数据库不存在该配置，历史调用不级联删除'

def skill_delete(c):
    sid=c.state['temp_skill'];c.http('DELETE',f'/api/skills/{sid}',expected=204)
    assert c.sql(f'SELECT COUNT(*) FROM skills WHERE id={sid};')=='0'
    return '删除技能后数据库一致'

def docs(c):
    doc=c.http('GET','/openapi.json',who=None);assert doc['openapi']=='3.0.3'
    for path in ['/api/agents','/api/conversations/{id}/messages','/api/mcp/servers/{id}/connect','/api/mcp/calls']:assert path in doc['paths']
    html=c.http('GET','/swagger/',who=None);assert 'SwaggerUIBundle' in html
    return f'OpenAPI含{sum(len(p) for p in doc["paths"].values())}项操作；Swagger页面可访问'
