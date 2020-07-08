//gcc cputime.c -o cputime
//time ./cputime
//time cgexec -g cpu:nick_cpu ./cputime

//
//[root@node ~]# time cgexec -g cpu:test_cpu ./cputime
//
//real	0m30.462s
//user	0m3.049s
//sys	0m0.005s
//
//[root@node ~]# time ./cputime
//
//real	0m2.907s
//user	0m2.895s
//sys	0m0.002s

void main()
{
    unsigned int i, end;

    end = 1024 * 1024 *1024;
    for(i = 0; i < end; )
    {
        i ++;
    }
}