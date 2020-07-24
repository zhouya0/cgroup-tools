#!/bin/bash
for proc in $(find /proc -maxdepth 1 -regex '/proc/[0-9]+'); do
    printf "%2d %5d %s\n" \
        "$(cat $proc/oom_score)" \
        "$(basename $proc)" \
        "$(cat $proc/cmdline | tr '\0' ' ' | head -c 50)"
done 2>/dev/null | sort -nr | head -n 40


#OOM分数值根据OOM_ADJ参数计算得出，对于Guaranteed级别的pod，OOM_ADJ参数设置成了-998，
#对于BestEffort级别的pod，OOM_ADJ参数设置成了1000，对于Burstable级别的POD，OOM_ADJ参数取值从2到999。对于kubernetes保留资源，
#比如kubelet，docker，OOM_ADJ参数设置成了-999，表示不会被OOM kill掉。OOM_ADJ参数设置的越大，通过OOM_ADJ参数计算出来OOM分数越高，
#表明该pod优先级就越低，当出现资源竞争时会越早被kill掉，对于OOM_ADJ参数是-999的表示kubernetes永远不会因为OOM而被kill掉。